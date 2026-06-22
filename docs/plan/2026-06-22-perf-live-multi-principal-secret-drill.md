# PERF Live Multi-Principal Secret Drill

## 1. Objective

Temporarily add enough static API keys/principals to the live `platform-gateway` runtime Secret to run the k6 `100` VU Project list test as `100` distinct principals, then restore the original Secret exactly. This aims to produce real `PERF-001` Project list and `PERF-002` 100-user evidence without disabling rate limiting.

## 2. Background

The k6 harness now supports `NEXUSPAAS_PERF_API_KEYS`, but the live `platform-gateway-runtime-secret` currently has only `1` `API_KEYS` entry and `1` `API_KEY_PRINCIPALS` entry. Because authenticated route rate limiting keys on verified `user:<id>`, one shared static API key hits the per-principal limiter and produced `429` responses.

## 3. Scope

Temporary live operation only:

- Backup current `platform-gateway-runtime-secret` values locally without printing them.
- Generate `100` high-entropy temporary static API keys.
- Generate matching `API_KEY_PRINCIPALS` entries with distinct IDs.
- Patch only `API_KEYS` and `API_KEY_PRINCIPALS` in `platform-gateway-runtime-secret`.
- Rollout restart only `deployment/platform-gateway`.
- Run k6 with the generated `NEXUSPAAS_PERF_API_KEYS` list from local process environment.
- Restore the original Secret values and rollout restart `platform-gateway`.
- Verify restored base64 `.data.API_KEYS` and `.data.API_KEY_PRINCIPALS` match the original captured bytes exactly, then also record restored counts.

Do not:

- Disable rate limiting.
- Patch backend code.
- Print, commit, or persist API key values.
- Touch non-gateway runtime Secrets.
- Leave temporary credentials active after the drill.

## 4. Principal Shape

`API_KEY_PRINCIPALS` is a JSON object keyed by the exact API key string. Each value must include a non-empty `id`; production validation requires a principal for every enabled API key.

Temporary principals:

- `id`: `perf-user-001` through `perf-user-100`
- `username`: same as `id`
- `role`: `service`
- `admin`: `false`
- `scopes`: `["platform:perf:read"]`

This is intentionally least-privilege for the read-only Project list route. If authorization policy denies this shape in preflight, stop and record the policy blocker rather than escalating to admin without a separate reviewed plan.

## 5. Evidence Rules

Record only:

- Original and restored key/principal counts.
- Secret name, namespace, and UID.
- Kubernetes context name after confirming it is the expected context.
- Rollout status.
- k6 exit status.
- `auth_key_count`.
- Per-endpoint request counts, failure rates, status counts, and p95.
- Whether any `429` responses occurred.

Never record:

- API key values.
- Secret JSON.
- Hashes of Secret values.
- Full decoded Secret data.

## 6. Verification Plan

1. Run the drill inside one non-interactive `set +x` shell block so Secret material stays in process environment/shell variables and is never printed.
2. Run a strict target preflight before mutation:
   - `kubectl config current-context` must equal the expected context for this live namespace.
   - Namespace must be exactly `nexuspaas`.
   - Secret name must be exactly `platform-gateway-runtime-secret`.
   - Capture and record Secret UID before mutation; fail closed if UID is empty.
   - `deployment/platform-gateway` must be ready before mutation.
3. Capture original `.data.API_KEYS` and `.data.API_KEY_PRINCIPALS` base64 strings into shell variables inside one `set +x` shell process; do not print values and do not write decoded values to disk.
4. Count original `API_KEYS` and `API_KEY_PRINCIPALS` and record counts only.
5. Generate temporary key/principal JSON locally inside the same shell process.
6. `kubectl -n nexuspaas patch secret platform-gateway-runtime-secret ...` with new base64-encoded `API_KEYS` and `API_KEY_PRINCIPALS`.
7. Verify the Secret UID did not change after patch.
8. `kubectl -n nexuspaas rollout restart deployment/platform-gateway`.
9. `kubectl -n nexuspaas rollout status deployment/platform-gateway --timeout=120s`.
10. Port-forward `svc/platform-gateway` to `127.0.0.1:18080`.
11. Run `make -C backend perf-k6-core-read` with `NEXUSPAAS_PERF_API_KEYS` in process environment.
12. Copy evidence numbers into this plan, `gap.md`, and `problem.md`.
13. Restore original base64 `.data.API_KEYS` and `.data.API_KEY_PRINCIPALS` exactly.
14. Rollout restart `platform-gateway` again and verify readiness.
15. Re-read restored base64 `.data.API_KEYS` and `.data.API_KEY_PRINCIPALS`; compare byte-for-byte with the original captured strings. Record only match booleans and restored counts.
16. Unset shell variables before exit. Do not leave local files containing Secret values; if any emergency file is created, it must be mode `0600`, removed before exit, and never included in evidence.
17. `git diff --check -- docs/plan/2026-06-22-perf-live-multi-principal-secret-drill.md gap.md problem.md`

Recorded evidence (2026-06-22 Asia/Taipei):

- Strict preflight passed before mutation: context `default`, namespace `nexuspaas`, Secret `platform-gateway-runtime-secret`, Secret UID `45c91d60-73f0-4ba0-b305-0f76db1dba7a`, gateway readiness `1/1`, original counts `API_KEYS=1` and `API_KEY_PRINCIPALS=1`.
- Generated `100` temporary static API keys and `100` matching least-privilege principals in process memory only.
- Patched only `API_KEYS` and `API_KEY_PRINCIPALS`; Secret UID remained unchanged.
- `deployment/platform-gateway` rollout after patch completed and returned readiness `1/1`.
- k6 ran with `auth_key_count=100`, but setup failed before load: `/api/v1/projects` preflight returned `403`, want `200`.
- k6/make status: k6 setup exception, `make` exit `2`. Summary existed with `auth_key_count=100`, total requests `3` from setup, total failure rate `0.3333333333333333`, total p95 `1.49ms`; no load endpoint requests were sent.
- The `403` means the least-privilege temporary principals are authenticated enough to reach policy, but do not have Project list authorization. Per plan, the drill stopped instead of escalating to admin.
- Original Secret base64 `.data.API_KEYS` and `.data.API_KEY_PRINCIPALS` were restored byte-for-byte: `api_keys_match=true`, `principals_match=true`.
- Restored counts returned to `API_KEYS=1` and `API_KEY_PRINCIPALS=1`; gateway readiness returned to `1/1`; Secret UID remained `45c91d60-73f0-4ba0-b305-0f76db1dba7a`.
- Result: rate limiting stayed enabled and temporary credentials were removed. `PERF-001`/`PERF-002` remain open because a true `100` principal Project list run is now blocked by authorization policy for least-privilege static principals, not by the k6 harness or lingering credentials.

## 7. Risk Controls

- If patch or rollout fails, immediately restore original Secret values and rollout restart.
- If k6 fails, still restore original Secret values before updating ledgers.
- If restore verification fails, stop further PERF work and record a blocker.
- Keep the test read-only and bounded by existing k6 guardrails.
- Keep `set +x` for the full shell block that holds Secret material; do not echo commands with Secret values.
- Do not use shell history for the generated command; run the operation as a non-interactive script block.

## 8. Acceptance Interpretation

- A pass requires `auth_key_count=100`, no 429s, global and per-endpoint failure thresholds pass, and `/api/v1/projects` p95 stays under `300ms`.
- If the test passes, this provides evidence for the Project list subset of `PERF-001` and for `PERF-002`.
- It still does not close `PERF-003..008`, nor other `PERF-001` API targets.

## 9. Reviewer Checklist

- [x] Secret handling avoids printing or persisting values.
- [x] Target context, namespace, Secret name, Secret UID, and gateway readiness are checked before mutation.
- [x] Original Secret base64 data is restored byte-for-byte after the drill.
- [x] Rate limiting remains enabled.
- [x] k6 uses 100 distinct temporary least-privilege principals.
- [x] Evidence records counts and outcomes without sensitive values.
- [x] Ledgers do not claim unrelated PERF items.

## 10. Status

Status: Implemented. Drill restored the original Secret exactly; `100` principal Project list evidence is blocked by authorization policy returning `403` for least-privilege temporary principals.
