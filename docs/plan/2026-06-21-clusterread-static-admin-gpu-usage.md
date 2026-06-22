# Clusterread Static Admin GPU Usage Compatibility Slice

## 1. Objective

Make the existing Project GPU usage read route accept the already-authenticated
static admin API key principal, so the live `/ui/` seeded E2E can prove
active-Project GPU usage without weakening client-supplied header protection.

## 2. Background

The latest seeded live Web GUI E2E creates a real Group/Project and proves the
active Project selector, ConfigFile submit, and Project image list. Its route
proof still records `GET /api/v1/projects/{id}/gpu-usage` as `HTTP 403`.

The route is implemented in `clusterread.getProjectGPUUsage`. It allows
project members or users whose projected read model grants adminPanel. Static
admin API keys are authenticated by platform middleware as `X-User-Role=admin`,
but this service does not yet treat that authenticated role header as admin
unless a projected identity row also exists.

## 3. Source References

- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `backend/internal/services/resourcehours/spec.go`
- `backend/internal/platform/auth.go`
- `backend/internal/platform/middleware.go`

## 4. Assumptions

- Live gateway/services run with `RequireAuth=true`.
- In auth-enabled requests, platform middleware strips inbound identity headers
  before applying static API key principal headers.
- Project GPU usage should stay readable only by project members or admins.

## 5. Non-Goals

- No new GPU usage endpoint.
- No fake GPU data or fixture endpoint.
- No change to project membership visibility.
- No OIDC, WebRTC, backup/restore, rollback, or performance work in this slice.
- No broad shared auth abstraction unless another approved slice proves the need.

## 6. Current Behavior

`GET /api/v1/projects/{id}/gpu-usage` returns `HTTP 403` for the live static
admin API key when the usage-observability read model lacks an adminPanel
identity projection for that principal.

## 7. Target Behavior

When `app.Config.RequireAuth` is true and the request carries
`X-User-Role=admin` from platform authentication,
`clusterread.hasAdminPanel` returns true. When `RequireAuth` is false, the same
header alone remains insufficient.

## 8. Affected Domains

- Usage-observability cluster read authorization.
- Web GUI active-Project Usage evidence.
- Acceptance ledgers.

## 9. Affected Files

- `backend/internal/services/clusterread/handler.go`
- `backend/internal/services/clusterread/handler_test.go`
- `backend/internal/services/clusterread/workflow_test.go`
- `docs/plan/2026-06-21-clusterread-static-admin-gpu-usage.md`
- `docs/plan/2026-06-21-web-gui-active-project-live-e2e.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None. Existing REST route, payload, and response shape remain unchanged.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No runtime logging or metrics change. Live E2E route proof should record HTTP
status and `used` count without logging API keys.

## 14. Security Considerations

- Trust role headers only when `RequireAuth=true`.
- Add a ServeHTTP spoofing test so a client-supplied admin role header without a
  valid admin API key cannot read Project GPU usage.
- Preserve project-member authorization and projected adminPanel behavior.
- Do not log secrets.

## 15. Implementation Steps

- [x] Add the smallest helper in `clusterread` to detect authenticated admin role
  headers only when `RequireAuth=true`.
- [x] Call that helper at the start of `clusterread.hasAdminPanel`.
- [x] Add direct helper/handler tests proving `RequireAuth=true` grants static
  admin access and `RequireAuth=false` does not.
- [x] Add a ServeHTTP spoofing test proving no-key and non-admin-key requests
  with spoofed `X-User-Role=admin` cannot read Project GPU usage.
- [x] Run focused clusterread tests and the seeded live Web GUI E2E.
- [x] Update ledgers honestly.

## 16. Verification Plan

Focused:

```sh
go -C backend test ./internal/services/clusterread -run 'StaticAdmin|Spoofed|ProjectGPU|ClusterProjectGPU' -count=1
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=/ui/ npm --prefix frontend run e2e
```

Regression:

```sh
go -C backend test ./internal/services/clusterread -count=1
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
npm --prefix frontend run test
npm --prefix frontend run build
git diff --check
```

If SonarScanner configuration or credentials are unavailable, record the exact
missing prerequisite as `Not Run`; final completion remains pending if this
slice requires Quality Gate evidence.

## 17. Rollback Plan

Revert this slice's clusterread helper/test/doc changes. The live GUI seeded
route proof would return to recording Project GPU usage `HTTP 403`.

## 18. Risks and Tradeoffs

- This repeats a small service-local compatibility check instead of adding a new
  shared authorization abstraction. That is intentional for this slice; add a
  shared helper only if a later approved slice needs the same code in more
  services.
- This proves route authorization and empty/non-empty GPU count behavior. It
  does not create real workload GPU telemetry.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: unblock active-Project GPU route for live seeded GUI E2E | Pass — seeded live E2E now records `gpu_status=200`, `gpu_ok=true` |
| Existing REST/OpenAPI only | Pass — no endpoint, payload, or transport change |
| No DB/migration/config change | Pass |
| Header trust limited to `RequireAuth=true` | Pass — helper accepts only authenticated `admin` role header |
| Spoofing test covers platform middleware boundary | Pass — no-key spoof returns 401 and reader-key spoof returns 403 through `ServeHTTP` |
| SOLID / service boundary fit | Pass — service-local compatibility helper; no shared abstraction added |
| 12-Factor compatibility | Pass — behavior follows runtime auth config and static API key principal config |
| Focused tests | Pass |
| Live seeded E2E | Pass |
| Sonar Quality Gate | Pass |
| Ledger accuracy | Pass — route is fixed, broader WEB/E2E remains partial |
| Diff scope | Pass — scoped to clusterread auth helper/tests and evidence docs |

## 20. Status

Status: Approved

Final implementation review: Approved by Reviewer Agent; no blocking findings.

Implementation evidence:

- `backend/internal/services/clusterread/handler.go` now treats
  `X-User-Role=admin` as adminPanel only when `app.Config.RequireAuth` is true.
- `backend/internal/services/clusterread/handler_test.go` proves
  `RequireAuth=false` does not trust the header, `RequireAuth=true` accepts
  `admin`, and `super-admin` is not accepted by this compatibility path.
- `backend/internal/services/clusterread/workflow_test.go` proves spoofed
  admin headers fail through `app.ServeHTTP` without a valid admin static API
  key, and a valid static admin key can read Project GPU usage.
- Rolled live `usage-observability-service` image:
  `localhost:5000/nexuspaas-backend:ci-ga-clusterread-static-admin-20260621132623`
  (`sha256:f6c6ab5badac315095c4ac299fb2ded4fac8c4f29ae910f65bed16eb9368a87f`).
- Live seeded E2E route proof after rollout:
  `project_count=1`, `seeded_project_present=true`, `config_file_count=1`,
  `seeded_config_id=CFG2600003`, `image_count=1`, `build_count=0`,
  `gpu_status=200`, `gpu_ok=true`.
- Live direct route probe after rollout:
  `project_gpu_usage_probe project=gpu-probe-p-1782019677-16791 status=200 used=0`.

Verification completed:

```sh
go -C backend test ./internal/services/clusterread -run 'StaticAdmin|Spoofed|ProjectGPU|ClusterProjectGPU' -count=1
go -C backend test ./internal/services/clusterread -count=1
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
npm --prefix frontend run test
npm --prefix frontend run build
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=/ui/ npm --prefix frontend run e2e
git diff --check
```

Plan Agent checklist:

- [x] Requirement restated.
- [x] Scope limited to existing clusterread Project GPU route.
- [x] Existing REST/OpenAPI contract preserved.
- [x] Security condition specified.
- [x] ServeHTTP spoofing test required.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
