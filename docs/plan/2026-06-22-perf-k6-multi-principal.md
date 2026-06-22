# PERF k6 Multi-Principal Core Read

## 1. Objective

Update the k6 core-read harness so `PERF-002` can be tested as `100` concurrent users/principals instead of `100` VUs sharing one static API key. Keep rate limiting enabled and use multiple configured principals for valid load evidence.

## 2. Background

The first live `100` VU core-read run failed because `/api/v1/projects` returned `58` HTTP `429` responses out of `333` load requests. Code inspection shows authenticated route rate limiting keys on verified user ID, and static API keys map to configured principals. A single API key therefore measures one principal under heavy concurrency, not `100` users.

Relevant code:

- `backend/internal/platform/middleware.go`: `ratelimit` guard returns `429` when `Rate.Allow(...)` denies.
- `backend/internal/platform/ratelimit.go`: auth-required routes key by verified `user:<id>`.
- `backend/internal/platform/api_key.go`: static API keys apply a configured principal into request auth context.
- `backend/internal/platform/app.go`: default limiter is `600` per minute.
- `docs/acceptance/performance.md`: `PERF-002` requires at least `100` concurrent users.

## 3. Scope

Implement:

- `NEXUSPAAS_PERF_API_KEYS`: comma-separated static API keys for multi-principal runs.
- Backward-compatible fallback to existing `NEXUSPAAS_PERF_API_KEY`.
- Deterministic protected-route key selection by `exec.vu.idInTest` so each VU consistently uses one key.
- Summary metadata with `auth_key_count`, never key values.
- Setup validation that fails before load if no auth key is present.

Do not implement:

- Product rate-limit bypass.
- Rate-limit config changes.
- Kubernetes Secret mutation in the harness.
- Logging, printing, or writing secret values.

## 4. Required Live Environment

To pass a true multi-principal `100` user run, the live namespace must provide enough configured static API keys, each mapped to a distinct `API_KEY_PRINCIPALS` ID. If the current live Secret contains only one key/principal, the harness implementation can be verified, but `PERF-002` remains blocked until the environment provides multi-principal auth material.

## 5. Design

- Parse `NEXUSPAAS_PERF_API_KEYS` by comma, trim blanks, and drop empty entries.
- If `NEXUSPAAS_PERF_API_KEYS` is empty, fall back to a one-item list from `NEXUSPAAS_PERF_API_KEY`.
- Do not deduplicate or print key values; duplicate keys are treated as caller configuration risk and surfaced only by `auth_key_count`.
- For protected endpoints, use `apiKeys[(exec.vu.idInTest - 1) % apiKeys.length]`.
- Setup preflight checks `/api/v1/projects` with every configured key to fail fast on invalid multi-principal input without logging key values.
- Existing per-endpoint failure, p95, status, and abort guardrails remain unchanged.

## 6. Acceptance And Evidence

- `PERF-002` evidence must record both `vus=100` and `auth_key_count`.
- If `auth_key_count < 100`, explicitly mark the evidence as not a full 100-principal pass even if the run succeeds.
- If 429s remain, record status counts and keep `PERF-001`/`PERF-002` open.
- If the live namespace has only one static API key, record the environment blocker instead of relaxing rate limits.

## 7. Implementation Steps

1. Update `backend/scripts/perf/core-read-100vu.js` to parse multiple env-provided keys.
2. Keep `backend/Makefile` unchanged.
3. Update `docs/plan/2026-06-22-perf-k6-core-read-100vu.md` with the multi-principal extension evidence.
4. Update `gap.md` and `problem.md` with the result or blocker.
5. Remove transient `.tmp` artifacts after evidence is copied into ledgers.

## 8. Verification Plan

- Missing auth check:
  - `make -C backend perf-k6-core-read` must fail before load when no key env is set.
- Single-key compatibility:
  - A short smoke with `NEXUSPAAS_PERF_API_KEY=<one key>` must still pass.
- Multi-key parser smoke:
  - A short smoke with `NEXUSPAAS_PERF_API_KEYS=<key1>,<key2>` must pass if live environment has two valid keys; otherwise record environment blocker.
- Live evidence:
  - Inspect live Secret key count without printing values.
  - If enough keys exist, run the `100` VU evidence with `NEXUSPAAS_PERF_API_KEYS`.
  - If not enough keys exist, stop and record that `PERF-002` needs multi-principal live credentials.
- Static checks:
  - `git diff --check -- backend/scripts/perf/core-read-100vu.js docs/plan/2026-06-22-perf-k6-multi-principal.md docs/plan/2026-06-22-perf-k6-core-read-100vu.md gap.md problem.md`

Recorded evidence (2026-06-22 Asia/Taipei):

- Missing auth fail-fast passed: `make -C backend perf-k6-core-read` failed during setup with `NEXUSPAAS_PERF_API_KEYS or NEXUSPAAS_PERF_API_KEY is required for /api/v1/projects`; no protected load traffic was sent.
- Single-key fallback smoke passed with `NEXUSPAAS_PERF_API_KEY`, `2` VUs, `3s`, and `NEXUSPAAS_PERF_THINK_TIME=0.1`: exit status `0`, `auth_key_count=1`, total requests `63` including preflight, total failure rate `0`, total p95 `3.09ms`; load endpoints each had `20` requests / `0` failures with p95 `/healthz=0.66ms`, `/readyz=0.74ms`, `/api/v1/projects=3.20ms`.
- `NEXUSPAAS_PERF_API_KEYS` parser smoke passed with one live key, `2` VUs, `3s`, and `NEXUSPAAS_PERF_THINK_TIME=0.1`: exit status `0`, `auth_key_count=1`, total requests `63` including preflight, total failure rate `0`, total p95 `3.46ms`; load endpoints each had `20` requests / `0` failures with p95 `/healthz=0.78ms`, `/readyz=0.98ms`, `/api/v1/projects=3.61ms`.
- Live `platform-gateway-runtime-secret` inspection counted `API_KEYS=1` and `API_KEY_PRINCIPALS=1` without printing values.
- Because the live namespace currently has only one static API key/principal, a real `100` principal `PERF-002` run is blocked by environment credentials. The harness is ready for multi-principal input, but `PERF-002` remains open until the live namespace provides enough distinct principals.

## 9. Security / 12-Factor

- All auth material remains env-only.
- No script reads Kubernetes Secrets.
- No key values, hashes, or decoded Secret material are written to docs or summary artifacts.
- Configuration stays environment-driven.

## 10. Reviewer Checklist

- [x] `PERF-002` is treated as concurrent principals/users, not one shared key.
- [x] Multi-key support is backward-compatible with `NEXUSPAAS_PERF_API_KEY`.
- [x] Key selection is deterministic and does not log secrets.
- [x] Rate limiting remains enabled.
- [x] Evidence records `auth_key_count` and keeps `PERF-001`/`PERF-002` open unless a real pass occurs.
- [x] If live credentials are insufficient, docs record a blocker rather than bypassing controls.

## 11. Status

Status: Implemented. Harness supports multi-principal input; live `100` principal evidence is blocked because current live credentials expose only one static key/principal.
