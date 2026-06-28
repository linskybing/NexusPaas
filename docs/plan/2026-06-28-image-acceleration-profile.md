# Image Acceleration Profile

## 1. Objective

Add the first image cold-start optimization slice: admin-managed
`ImageAccelerationProfile` records in `image-registry-service`, with seeded
defaults and contract tests.

## 2. Background

The AI/HPC optimization roadmap calls out image cold-start as the next platform
gap after storage, network, placement, and accelerator profiles. The repository
already uses generic record storage and per-service `Spec()` route contracts for
similar profile entities.

This slice adds policy metadata only. It does not implement image conversion,
node prewarm execution, or a custom lazy-pull controller.

## 3. Source References

- `backend/internal/services/imageregistry/spec.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/schedulerquota/network_profiles.go`
- `backend/internal/services/schedulerquota/placement_profiles.go`
- `backend/internal/services/storage/storage_profiles.go`
- `backend/internal/contracts/fixtures/api/v1/scheduler-create-network-profile.json`
- `backend/internal/contracts/fixtures/events/v1/network-profile-changed.json`
- `docs/acceptance/image-build.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 4. Assumptions

- `image-registry-service` owns image acceleration profile metadata.
- Generic `platform_records` remains the durable store; no SQL DDL is needed.
- Admin CRUD and seeded defaults are enough for the first reviewable slice.
- Runtime consumers can be added later once workload/image dispatch needs the
  profile.

## 5. Non-Goals

- No eStargz, Nydus, SOCI, or BuildKit conversion implementation.
- No prewarm DaemonSet/controller.
- No registry mirror provisioning.
- No workload dispatch changes.
- No frontend changes.
- No live Harbor execution proof.

## 6. Current Behavior

- Image-registry service manages Harbor catalog sync, image requests, allow
  lists, build jobs, and Harbor health/status.
- There is no first-class profile describing lazy snapshotter, prewarm policy,
  conversion requirement, or allowed project scope.
- The image-build acceptance row remains open for cold-start and external image
  workflow maturity.

## 7. Target Behavior

- Admins can create, list, get, update, and delete image acceleration profiles.
- Defaults are seeded idempotently:
  - `standard-overlayfs`: standard overlayfs, no prewarm, no conversion.
  - `estargz-gpu-prewarm`: stargz snapshotter, nodepool prewarm, conversion
    required.
  - `soci-inference-prewarm`: SOCI snapshotter, queue/project prewarm,
    conversion required.
- `ImageAccelerationProfileChanged` is emitted for profile mutations.
- Required fields are `name`, `snapshotter`, and `prewarm_policy`.

## 8. Affected Domains

- `image-registry-service`: owns image acceleration profile metadata and events.
- Contract fixtures: external REST create contract and event envelope fixture.

## 9. Affected Files

- `backend/internal/services/imageregistry/spec.go`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/image_acceleration_profiles.go`
- `backend/internal/services/imageregistry/image_acceleration_profiles_test.go`
- `backend/internal/services/imageregistry/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/image-registry-create-acceleration-profile.json`
- `backend/internal/contracts/fixtures/events/v1/image-acceleration-profile-changed.json`
- `docs/acceptance/image-build.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 10. API / Contract Changes

Add admin CRUD:

- `GET /api/v1/image-acceleration-profiles`
- `POST /api/v1/image-acceleration-profiles`
- `GET /api/v1/image-acceleration-profiles/{id}`
- `PUT /api/v1/image-acceleration-profiles/{id}`
- `DELETE /api/v1/image-acceleration-profiles/{id}`

Add event:

- `ImageAccelerationProfileChanged`

## 11. Database / Migration Changes

No SQL migrations. New durable generic record resource:

- `image-registry-service:image_acceleration_profiles`

## 12. Configuration Changes

None.

## 13. Observability Changes

Profile mutations emit `ImageAccelerationProfileChanged` through the existing
outbox path. No metrics are added in this slice.

## 14. Security Considerations

- Profile writes are admin-only.
- Profiles must not store credentials or registry secrets.
- User-submitted workload/image payloads are not trusted to inject arbitrary
  node prewarm commands in this slice.

## 15. Implementation Steps

1. Add the profile resource, seed helper, custom create/update/delete handlers,
   required-field validation, and event payload helper.
2. Register seed and custom handlers in `imageregistry.Register`.
3. Add routes, table, and event to `imageregistry.Spec()`.
4. Add focused tests for idempotent seed, required-field rejection, admin guard,
   and event emission.
5. Add API and event fixtures plus image-registry fixture validation.
6. Update acceptance docs to record local metadata/contract evidence only.

## 16. Verification Plan

- `cd backend && go test ./internal/services/imageregistry -run "ImageAcceleration|ExternalAPI"`
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./internal/services/...`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`
- `git diff --check`

## 17. Rollback Plan

Remove the new image acceleration profile routes, handlers, seed helper, tests,
fixtures, and docs. Existing generic records become inert if any were created.

## 18. Risks and Tradeoffs

- This does not reduce cold-start by itself; it only creates the policy contract
  for later image conversion/prewarm work.
- Seeding only conservative defaults avoids pretending unsupported runtimes are
  installed.
- Custom handlers add a little code, but keep event names consistent with the
  existing profile patterns.

## 19. Reviewer Checklist

- Scope is limited to image acceleration profile metadata.
- No image conversion or prewarm execution is implemented.
- Routes/tables/events are declared in `Spec()` and fixtures.
- Admin guard and required fields are tested.
- Acceptance docs keep image workflow and cold-start execution gaps open.

## 20. Status

Status: Approved
