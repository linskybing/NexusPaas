# Storage Cache / Benchmark Ledger Sync Plan

Date: 2026-06-28
Status: Ready for Reviewer Agent
Scope: docs/evidence-only ledger sync

## Objective

Sync already-implemented storage CacheBinding and StorageBenchmarkRecord
evidence into the project ledgers. This slice updates evidence wording only
after Reviewer approval. It does not change runtime code, tests, routes,
events, fixtures, or Kubernetes manifests.

## Current Evidence

- CacheBinding exists in `backend/internal/services/storage/cache_bindings.go`
  with resource `storage-service:cache_bindings`, project-manager scoped CRUD,
  and `CacheBindingChanged` events.
- CacheBinding coverage exists in
  `backend/internal/services/storage/cache_binding_test.go`.
- DataPlanePlan cache-hit behavior from CacheBinding exists in
  `TestDataPlanePlanMarksCacheHitFromCacheBinding`.
- StorageBenchmarkRecord exists in
  `backend/internal/services/storage/benchmark_records.go` with resource
  `storage-service:storage_benchmark_records`, required `storage_profile`, and
  `StorageBenchmarkRecorded` events.
- StorageBenchmarkRecord coverage exists in
  `backend/internal/services/storage/benchmark_record_test.go`.
- Contract fixtures already exist:
  `backend/internal/contracts/fixtures/api/v1/storage-create-cache-binding.json`,
  `backend/internal/contracts/fixtures/api/v1/storage-create-benchmark-record.json`,
  and
  `backend/internal/contracts/fixtures/api/v1/storage-list-benchmark-records.json`.

## Affected Files

Plan file:
- `docs/plan/2026-06-28-storage-cache-benchmark-ledger-sync.md`

Post-approval ledger updates only:
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## Exact Ledger Wording Boundaries

Use bounded wording equivalent to:

> Storage CacheBinding and StorageBenchmarkRecord now have local/static storage-service evidence: CacheBinding project-manager scoped CRUD and DataPlanePlan cache-hit marking from an existing CacheBinding are covered by focused storage tests; `CacheBindingChanged` is implemented by the handler and declared in service Spec/API/event fixtures; StorageBenchmarkRecord create/list behavior, required `storage_profile`, and `StorageBenchmarkRecorded` event emission are covered by focused storage tests, with typed create/list fixture coverage in the contracts suite. This is local/static storage metadata evidence only; it does not prove live cache residency, node-local NVMe reuse, cache eviction, live benchmark execution, fio/IOR/NCCL measurement collection, performance baselines, Kubernetes storage backend behavior, storage GA, Full GA, or V1 external production launch readiness.

Ledger placement:
- In `gap.md`, add one Done-table row or a short dated paragraph near the
  existing storage/data-plane evidence rows.
- In `problem.md`, add the same bounded evidence note near existing storage
  and image/scheduler local-static evidence paragraphs.
- In `docs/acceptance/gap-analysis.md`, add the same bounded evidence note in
  the current implementation status narrative near storage/data-plane evidence.

## Verification Commands

Run from `backend/` unless noted:

```bash
go test ./internal/services/storage -run 'CacheBinding|StorageBenchmark'
go test ./internal/contracts/...
```

Run from repo root:

```bash
git diff --check
```

## Non-Goals

- No runtime code changes.
- No new routes, events, fixtures, or migrations.
- No live kind/RKE2/Kubernetes execution claim.
- No Full GA, storage GA, or V1 external production launch claim.
- No benchmark-result generation or performance acceptance claim.
- No claim that CacheBinding proves actual node-local cache residency.

## Reviewer Notes

This is intentionally a ledger sync for evidence that already exists in code and
tests. Reviewer should verify the wording stays bounded to local/static evidence
and that no runtime files are modified in this slice.
