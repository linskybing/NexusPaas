# PERF Live Baseline

## 1. Objective

Record a small live performance baseline so the GA tracker has real latency evidence instead of an empty `PERF` row, without claiming `PERF-001..008` are complete.

## 2. Background

`docs/acceptance/performance.md` defines full GA performance acceptance, including 100 concurrent users and several workload, usage, WebRTC, build, and K8s-control stress tests. The current tracker says `PERF` has no load-test evidence. A baseline is useful, but it must not be confused with the full performance suite.

## 3. Source References

- `docs/acceptance/performance.md`
- `gap.md`
- `problem.md`
- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`

## 4. Assumptions

- A short read-only baseline against the current live gateway is safe.
- `k6` is available locally as `/snap/bin/k6` and can be used as a one-off tool without adding a repository dependency.
- Protected write/workload/WebRTC/build stress tests are out of scope for this slice.

## 5. Non-Goals

- Do not claim `PERF-001..008` are complete.
- Do not add k6 scripts to the repository or a new package dependency.
- Do not run workload admission, image build, WebRTC, or K8s-control stress.
- Do not print tokens, cookies, API keys, or decoded Kubernetes Secret values.

## 6. Current Behavior

`gap.md` lists `PERF` as no load-test evidence, and `performance.md` is unverified.

## 7. Target Behavior

`gap.md` records one bounded live baseline with exact endpoint set, concurrency, request count, per-endpoint p95 latency, per-endpoint failure rate, preflight status-code summary, and clear remaining PERF gaps.

## 8. Affected Domains

- Documentation and GA evidence only.
- Live gateway read-only routes for measurement.

## 9. Affected Files

- `docs/plan/2026-06-22-perf-live-baseline.md`
- `gap.md`
- `problem.md`

Temporary artifacts are written under `.tmp/nexuspaas-perf-baseline-*` because the installed snap-packaged k6 cannot read the system `/tmp`; artifacts are removed after evidence is copied into the ledgers.

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

The baseline records client-side latency and HTTP status counts only.

## 14. Security Considerations

- Use read-only endpoints:
  - `GET /healthz`
  - `GET /readyz`
  - `GET /service-registry`
  - `GET /api/v1/projects`
  - `GET /outbox`
  - `GET /projections`
  - `GET /ui/`
- Pass the existing static API key to k6 through `NEXUSPAAS_PERF_API_KEY` for protected read-only routes; do not print the key or decoded Secret values.
- Keep request volume low enough to avoid harming the shared live namespace.

## 15. Implementation Steps

1. Port-forward `platform-gateway` locally only for the baseline.
2. Decode the live static API key into process environment only:
   - `NEXUSPAAS_PERF_API_KEY=<decoded in shell without printing>`
3. Write `.tmp/nexuspaas-perf-baseline.js` with:
   - `vus: 20`
   - `iterations: 210`
   - timeout `5s`
   - deterministic endpoint sequence `['/healthz', '/readyz', '/service-registry', '/api/v1/projects', '/outbox', '/projections', '/ui/']`
   - endpoint selected by `exec.scenario.iterationInTest % endpoints.length`; do not use VU-local `__ITER` because it does not guarantee equal endpoint counts under shared iterations
   - threshold `http_req_failed < 0.01`
   - per-endpoint request-count submetrics using `http_reqs{endpoint:<path>}` with `count==30`
   - per-endpoint duration submetrics using `http_req_duration{endpoint:<path>}` with a lenient `p(95)<5000` smoke threshold
   - summary export `.tmp/nexuspaas-perf-baseline-summary.json`
   - `handleSummary()` writes `.tmp/nexuspaas-perf-baseline-endpoints.json` with per-endpoint request count, failure rate, and p95 latency.
4. Run k6 through a local port-forward.
5. Run one preflight HTTP status probe per endpoint with the same auth rules.
6. Record preflight status codes, request counts, failure rates, and p95 latency per endpoint.
7. Update `gap.md` and `problem.md` to replace "no load-test evidence" with accurate baseline evidence and explicitly keep each `PERF-001..008` open.

## 16. Verification Plan

- `mkdir -p .tmp && cat > .tmp/nexuspaas-perf-baseline.js <<'EOF' ... EOF`
- `kubectl -n nexuspaas port-forward svc/platform-gateway 8080:80`
- `NEXUSPAAS_PERF_API_KEY=<decoded without printing> k6 run --summary-export .tmp/nexuspaas-perf-baseline-summary.json .tmp/nexuspaas-perf-baseline.js`
- `NEXUSPAAS_PERF_API_KEY=<decoded without printing> python3 - <<'PY' ... preflight status probe ... PY`
- `rm -f .tmp/nexuspaas-perf-baseline.js .tmp/nexuspaas-perf-baseline-summary.json .tmp/nexuspaas-perf-baseline-endpoints.json`
- `git diff --check -- docs/plan/2026-06-22-perf-live-baseline.md gap.md problem.md`

Recorded evidence (2026-06-22 Asia/Taipei):

- `k6 v1.6.1` was available locally and used as a one-off tool; no repository dependency was added.
- Initial live baseline used `vus=20`, `iterations=210`, timeout `5s`, and deterministic endpoint sequence `/healthz`, `/readyz`, `/service-registry`, `/api/v1/projects`, `/outbox`, `/projections`, `/ui/`. It completed all `210` iterations but exited `99` because smoke thresholds failed: failure rate `4.2857%`, total p95 `4182.04ms`, and repeated `/outbox` `5s` request timeouts with `/outbox` p95 `5000.49ms`.
- A retained-counter rerun first proved VU-local `__ITER` was the wrong selector because `http_reqs{endpoint:<path>} count==30` thresholds failed. The script was corrected to use `exec.scenario.iterationInTest % endpoints.length`.
- Corrected retained-counter rerun completed all `210` iterations and exited `0`: total failure rate `0`, total p95 `2306.00ms`.
- Corrected per-endpoint result: `/healthz` `30` requests, `0` failures, p95 `305.39ms`; `/readyz` `30` requests, `0` failures, p95 `766.16ms`; `/service-registry` `30` requests, `0` failures, p95 `493.44ms`; `/api/v1/projects` `30` requests, `0` failures, p95 `1161.30ms`; `/outbox` `30` requests, `0` failures, p95 `3118.15ms`; `/projections` `30` requests, `0` failures, p95 `816.28ms`; `/ui/` `30` requests, `0` failures, p95 `475.13ms`.
- A follow-up single-request preflight after restarting port-forward returned `/healthz 200 5.8ms`, `/readyz 200 1.2ms`, `/service-registry 200 1.1ms`, `/api/v1/projects 200 4.7ms`, `/outbox 200 118.7ms`, then `/projections` and `/ui/` timed out after the port-forward stream became unhealthy. This is recorded as port-forward/live-read-path instability, not proof that `/projections` or `/ui/` are intrinsically broken.

## 17. Rollback Plan

Revert the documentation updates. No runtime change is made.

## 18. Risks and Tradeoffs

- This baseline is intentionally too small to satisfy full GA performance acceptance.
- It reduces unknowns for gateway/UI read paths only.
- Thresholds are baseline smoke gates only; they are not the acceptance targets in `docs/acceptance/performance.md`.
- It uses the existing static API key for protected read-only routes because unauthenticated protected routes correctly return 401.

## 19. Reviewer Checklist

- [x] Requirement fit: moves PERF from no evidence to bounded baseline evidence.
- [x] Scope is limited to docs and read-only live measurement.
- [x] Endpoint list, concurrency, request count, timeout, output path, and threshold are deterministic.
- [x] No dependency or framework added to the repo.
- [x] No secret logging.
- [x] Ledgers clearly keep every `PERF-001..008` item open.

## 20. Status

Status: Implemented. Baseline evidence recorded; full `PERF-001..008` acceptance remains open.
