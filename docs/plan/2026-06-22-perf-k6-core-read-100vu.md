# PERF k6 Core Read 100 VU Harness

## 1. Objective

Add a reusable k6 harness for a bounded `100` VU live read test of gateway health/readiness and the protected Project list API, then run it once against the current live namespace to advance `PERF-001` and `PERF-002` evidence without claiming full `PERF-001..008` completion.

## 2. Source References

- `docs/acceptance/performance.md`
- `docs/plan/2026-06-22-perf-live-baseline.md`
- `gap.md`
- `problem.md`
- `backend/Makefile`
- `backend/docs/non-functional-requirements.md`

## 3. Acceptance Scope

- Advances `PERF-001` only for the `Project list p95 <= 300 ms` core API target.
- Advances `PERF-002` by running with `100` concurrent k6 virtual users.
- Does not cover login, ConfigFile submit, manifest preflight, queue admission, job status, usage query, image build, stream credential, build concurrency, WebRTC concurrency, metrics cardinality, or K8s-control throughput.
- `PERF-001..008` remain open unless their full criteria are separately evidenced.

## 4. Design

Use k6 rather than a custom load generator.

Add:

- `backend/scripts/perf/core-read-100vu.js`
- `backend/Makefile` target `perf-k6-core-read`

The harness:

- Uses `constant-vus` with `vus=100` by default.
- Runs for `30s` by default.
- Sends read-only requests to:
  - `GET /healthz`
  - `GET /readyz`
  - `GET /api/v1/projects`
- Uses `exec.scenario.iterationInTest` for deterministic endpoint selection.
- Requires `NEXUSPAAS_PERF_API_KEYS` or fallback `NEXUSPAAS_PERF_API_KEY` for `/api/v1/projects` and fails before load starts if no auth key is present.
- Reads base URL from `NEXUSPAAS_PERF_BASE_URL`, defaulting only to local `http://127.0.0.1:18080`.
- Emits a machine-readable endpoint summary with request count, failure rate, failures, and p95 latency.
- Emits per-endpoint status counters for `2xx`, `4xx`, `5xx`, exact `429`, and `other`.
- Runs preflight requests before the 100 VU scenario and aborts before load if any endpoint returns a bad status.
- Tags preflight requests separately so load endpoint p95 and request counts are not biased by setup checks.
- Aborts the run if the global failure rate crosses a conservative guardrail during the run.

## 5. Configuration

Environment variables:

- `NEXUSPAAS_PERF_BASE_URL` defaults to `http://127.0.0.1:18080`.
- `NEXUSPAAS_PERF_API_KEYS` or fallback `NEXUSPAAS_PERF_API_KEY` is required for protected routes.
- `NEXUSPAAS_PERF_VUS` defaults to `100`.
- `NEXUSPAAS_PERF_DURATION` defaults to `30s`.
- `NEXUSPAAS_PERF_TIMEOUT` defaults to `5s`.
- `NEXUSPAAS_PERF_THINK_TIME` defaults to `1s` to keep shared live namespace pressure bounded.
- `NEXUSPAAS_PERF_SUMMARY_PATH` defaults to `.tmp/nexuspaas-perf-core-read-100vu-endpoints.json`.

No live URL, namespace, API key, or decoded Kubernetes Secret value is committed.

## 6. Thresholds

- Global failure rate: `http_req_failed < 0.01`.
- Per-endpoint failure rate: `http_req_failed{endpoint:<path>} < 0.01`.
- Per-endpoint request counters: `http_reqs{endpoint:<path>} count>0` to force k6 submetrics into the summary.
- Per-endpoint status counters: custom k6 counters for `2xx`, `4xx`, `5xx`, exact `429`, and `other`.
- `/api/v1/projects` p95 target-tracking threshold: `http_req_duration{endpoint:/api/v1/projects} p(95)<300`.
- `/healthz` p95 smoke threshold: `http_req_duration{endpoint:/healthz} p(95)<1000`.
- `/readyz` p95 smoke threshold: `http_req_duration{endpoint:/readyz} p(95)<1000`.
- Guardrail abort: `http_req_failed rate<0.05` with `abortOnFail=true` and a short delay so the shared live namespace is not hammered when transport/auth failures spike.

If any threshold fails, the run is still valid evidence, but the ledgers must record it as failed or partial evidence.

## 7. Live-Run Guardrails

- Preflight must run once before the load scenario:
  - `/healthz` must return `200`.
  - `/readyz` must return `200`.
  - `/api/v1/projects` must return `200` with configured `NEXUSPAAS_PERF_API_KEYS` or fallback `NEXUSPAAS_PERF_API_KEY`.
- Missing auth keys must throw a clear setup error before any protected request is sent.
- If preflight fails, do not start the 100 VU scenario; record the setup failure instead.
- The k6 threshold guardrail must abort if failures cross `5%` after the first short window.

## 8. Implementation Steps

1. Add `backend/scripts/perf/core-read-100vu.js`.
2. Add `perf-k6-core-read` to `backend/Makefile` help and `.PHONY`.
3. Keep the Makefile target thin: it creates the default `.tmp` artifact directory and invokes `k6 run backend/scripts/perf/core-read-100vu.js`.
4. Run syntax/smoke verification with minimal VUs and duration against a local or live gateway.
5. Run live `100` VU evidence with `NEXUSPAAS_PERF_API_KEYS` or fallback `NEXUSPAAS_PERF_API_KEY` loaded from local process environment without printing values.
6. Record exit status, endpoint request counts, failure rates, p95 values, status counters, and threshold result in this plan plus `gap.md` and `problem.md`.
7. Remove transient `.tmp` artifacts after copying evidence into the ledgers.

## 9. Verification Plan

- `make -C backend perf-k6-core-read` with env overrides for a short smoke run.
- Live run:
  - `kubectl -n nexuspaas port-forward svc/platform-gateway 18080:80`
  - `NEXUSPAAS_PERF_API_KEYS=<decoded list without printing> make -C backend perf-k6-core-read`
- Confirm `perf-k6-core-read` is not referenced by `backend/scripts/ci-security-gate.sh` or `.github/workflows/backend-quality-gate.yml`.
- `git diff --check -- backend/Makefile backend/scripts/perf/core-read-100vu.js docs/plan/2026-06-22-perf-k6-core-read-100vu.md gap.md problem.md`

Recorded evidence (2026-06-22 Asia/Taipei):

- Missing auth fail-fast check passed: `make -C backend perf-k6-core-read` failed during setup with `NEXUSPAAS_PERF_API_KEYS or NEXUSPAAS_PERF_API_KEY is required for /api/v1/projects` and did not send protected load traffic.
- Short live smoke passed with `NEXUSPAAS_PERF_VUS=2`, `NEXUSPAAS_PERF_DURATION=3s`, `NEXUSPAAS_PERF_THINK_TIME=0.1`, and `NEXUSPAAS_PERF_SUMMARY_PATH=.tmp/nexuspaas-perf-core-read-smoke-endpoints.json`: exit status `0`, total requests `63` including preflight, total failure rate `0`, total p95 `3.05ms`; load endpoints each had `20` requests / `0` failures with p95 `/healthz=0.67ms`, `/readyz=1.03ms`, `/api/v1/projects=3.86ms`.
- Live `100` VU run used defaults `NEXUSPAAS_PERF_VUS=100`, `NEXUSPAAS_PERF_DURATION=30s`, `NEXUSPAAS_PERF_THINK_TIME=1`, timeout `5s`, against local port-forward `http://127.0.0.1:18080`.
- The `100` VU run started cleanly but guardrail-aborted after about `10s`: k6 exited `99` and `make` exited `2` because `http_req_failed` and `http_req_failed{endpoint:/api/v1/projects}` crossed thresholds.
- `100` VU summary: total requests `1003` including preflight, total failure rate `0.05782652043868395`, total p95 `9.49ms`.
- Load endpoint results: `/healthz` `334` requests / `0` failures / `334` `2xx` / p95 `7.11ms`; `/readyz` `333` requests / `0` failures / `333` `2xx` / p95 `7.44ms`; `/api/v1/projects` `333` requests / `58` failures / failure rate `0.17417417417417416` / `275` `2xx` / `58` `429` / p95 `12.30ms`.
- Immediate post-run probe returned `/healthz 200 5.69ms`, `/readyz 200 1.14ms`, and `/api/v1/projects 429 1.11ms`; a later recovery probe returned `/api/v1/projects 200` on the first retry.
- `k6 version` returned `k6 v1.6.1`.
- `rg -n "perf-k6-core-read" backend/scripts/ci-security-gate.sh .github/workflows/backend-quality-gate.yml` returned no CI references.
- `git diff --check -- backend/Makefile backend/scripts/perf/core-read-100vu.js docs/plan/2026-06-22-perf-k6-core-read-100vu.md gap.md problem.md` passed.
- Transient `backend/.tmp/nexuspaas-perf-core-read-*` summary artifacts were removed after copying evidence into ledgers.
- Result: the harness is implemented and live-evidenced, but the `100` VU Project list acceptance evidence failed because static-key single-client load hit rate limiting. `PERF-001` Project list latency target cannot be accepted despite low p95 because the endpoint did not sustain the run without 429s. `PERF-002` has a failed 100 VU evidence run, not a pass.
- Follow-up plan `docs/plan/2026-06-22-perf-k6-multi-principal.md` updated the harness to support `NEXUSPAAS_PERF_API_KEYS` and record `auth_key_count`. Live verification found only `1` static key/principal in `platform-gateway-runtime-secret`, so true `100` principal evidence remains blocked by environment credentials rather than a harness limitation.

## 10. Security And 12-Factor Checks

- Static API key is provided only by environment variable.
- No script reads Kubernetes Secrets directly.
- No key, token, cookie, or decoded Secret value is printed.
- Runtime target and tuning are environment-driven.
- The harness is stateless and writes only the configured local summary artifact.

## 11. SOLID / Design Checks

- Single responsibility: one k6 file covers one read-only core API load scenario.
- Open/closed: scenario tuning is env-driven, not hardcoded per environment.
- Dependency inversion: the harness depends on HTTP/API contracts and env config, not Kubernetes internals.
- No custom load-test framework is introduced.

## 12. Reviewer Checklist

- [x] Requirement fit: advances only `PERF-001` Project list and `PERF-002` 100 VU evidence.
- [x] Does not claim `PERF-001..008` completion.
- [x] Secrets are env-only and never logged.
- [x] k6 script has deterministic endpoint tagging and machine-readable summary output.
- [x] Summary includes status counters sufficient to identify rate limiting.
- [x] Per-endpoint thresholds are explicit for `/healthz`, `/readyz`, and `/api/v1/projects`.
- [x] Preflight and abort guardrails bound risk to the shared live namespace.
- [x] Makefile target is optional and not added to default CI gates.
- [x] Live evidence is recorded accurately, including failures if thresholds fail.

## 13. Status

Status: Implemented. Harness added; live `100` VU Project list evidence failed due `429` rate limiting, so `PERF-001`/`PERF-002` remain open.
