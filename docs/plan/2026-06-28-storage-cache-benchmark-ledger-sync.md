# Storage Cache / Benchmark Ledger Sync Plan

Date: 2026-06-28
Scope: docs/evidence-only ledger sync

## 1. Objective

Sync already-implemented storage CacheBinding and StorageBenchmarkRecord
evidence into the project ledgers. This slice updates evidence wording only.
It does not change runtime code, tests, routes, events, fixtures, or Kubernetes
manifests.

## 2. Background

CacheBinding and StorageBenchmarkRecord are implemented and tested with
fixtures, but the project ledgers do not yet record that evidence, leaving the
trackers out of sync with the working tree.

## 3. Source References

- `backend/internal/services/storage/cache_bindings.go`
- `backend/internal/services/storage/cache_binding_test.go`
- `backend/internal/services/storage/benchmark_records.go`
- `backend/internal/services/storage/benchmark_record_test.go`
- `backend/internal/contracts/fixtures/api/v1/storage-create-cache-binding.json`
- `backend/internal/contracts/fixtures/api/v1/storage-create-benchmark-record.json`
- `backend/internal/contracts/fixtures/api/v1/storage-list-benchmark-records.json`

## 4. Assumptions

- The cited code and tests are already implemented and passing.
- Updating evidence wording does not change runtime behavior or acceptance scope.

## 5. Non-Goals

- No runtime code changes.
- No new routes, events, fixtures, or migrations.
- No live kind/RKE2/Kubernetes execution claim.
- No Full GA, storage GA, or V1 external production launch claim.
- No benchmark-result generation or performance acceptance claim.
- No claim that CacheBinding proves actual node-local cache residency.

## 6. Current Behavior

- CacheBinding exists in `cache_bindings.go` with resource
  `storage-service:cache_bindings`, project-manager scoped CRUD, and
  `CacheBindingChanged` events; covered by `cache_binding_test.go`.
- DataPlanePlan cache-hit behavior from CacheBinding exists in
  `TestDataPlanePlanMarksCacheHitFromCacheBinding`.
- StorageBenchmarkRecord exists in `benchmark_records.go` with resource
  `storage-service:storage_benchmark_records`, required `storage_profile`, and
  `StorageBenchmarkRecorded` events; covered by `benchmark_record_test.go`.
- Contract fixtures already exist for create-cache-binding,
  create-benchmark-record, and list-benchmark-records.
- None of this is reflected in `gap.md`, `problem.md`, or
  `docs/acceptance/gap-analysis.md`.

## 7. Target Behavior

The three ledgers carry one bounded evidence note for CacheBinding and
StorageBenchmarkRecord metadata, scoped to local/static evidence.

## 8. Affected Domains

- Repository documentation and acceptance ledgers only.

## 9. Affected Files

- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

No secrets or operational evidence values are added.

## 15. Implementation Steps

1. In `gap.md`, add one Done-table row or short dated paragraph near the existing
   storage/data-plane evidence rows.
2. In `problem.md`, add the same bounded evidence note near existing storage and
   image/scheduler local-static evidence paragraphs.
3. In `docs/acceptance/gap-analysis.md`, add the same bounded note in the current
   implementation status narrative near storage/data-plane evidence.

Ledger boundary wording:

> Storage CacheBinding and StorageBenchmarkRecord now have local/static
> storage-service evidence: CacheBinding project-manager scoped CRUD and
> DataPlanePlan cache-hit marking from an existing CacheBinding are covered by
> focused storage tests; `CacheBindingChanged` is implemented by the handler and
> declared in service Spec/API/event fixtures; StorageBenchmarkRecord create/list
> behavior, required `storage_profile`, and `StorageBenchmarkRecorded` event
> emission are covered by focused storage tests, with typed create/list fixture
> coverage in the contracts suite. This is local/static storage metadata evidence
> only; it does not prove live cache residency, node-local NVMe reuse, cache
> eviction, live benchmark execution, fio/IOR/NCCL measurement collection,
> performance baselines, Kubernetes storage backend behavior, storage GA, Full
> GA, or V1 external production launch readiness.

## 16. Verification Plan

Run from `backend/`:

```bash
go test ./internal/services/storage -run 'CacheBinding|StorageBenchmark'
go test ./internal/contracts/...
```

Run from repo root:

```bash
git diff --check
```

## 17. Rollback Plan

Revert the bounded evidence wording in the three ledger files. No runtime
rollback is needed.

## 18. Risks and Tradeoffs

- The risk is overclaiming; keep the wording bounded to local/static evidence
  and modify no runtime files.

## 19. Reviewer Checklist

- This is intentionally a ledger sync for evidence that already exists in code
  and tests.
- Reviewer should verify the wording stays bounded to local/static evidence and
  that no runtime files are modified in this slice.

## 20. Status

Status: Approved
