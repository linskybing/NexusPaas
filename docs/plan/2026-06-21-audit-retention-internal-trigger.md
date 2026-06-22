# Audit Retention Internal Trigger

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Reduce the `AUDIT-001..004` gap by making audit-log retention enforceable
through the existing service-internal cleanup route, with focused tests and
ledger evidence.

## Current Evidence

- `gap.md` marks audit as partial because group-scoped query and enforced
  retention remain open.
- `CleanupOldAuditLogs` already deletes `audit_logs` older than
  `AuditRetentionDays`, defaults unset retention to 30 days, and is wired as a
  lease-gated maintenance task.
- `Spec()` already declares
  `POST /api/v1/internal/audit/cleanup` as an admin, service-internal route with
  `ServiceAuthRequired` and `PolicyBypass`.
- `Register()` does not register a custom handler for the internal cleanup
  route, so there is no direct operational trigger/evidence path for retention
  outside the background maintenance cycle.

## Scope

- Register a custom handler for `POST /api/v1/internal/audit/cleanup`.
- Reuse `CleanupOldAuditLogs`; do not add a second deletion implementation.
- Keep the existing route metadata unchanged: service-internal auth required,
  policy bypass for internal traffic, and no public/authenticated user access.
- Return a small JSON payload with at least `removed` and `retention_days` so
  operators and tests can prove enforcement happened.
- Add focused audit-compliance tests for:
  - route metadata remains service-internal and policy-bypassed,
  - missing or invalid service API key cannot trigger cleanup,
  - valid service API key deletes only expired audit logs,
  - unset retention reports and uses the 30-day default.
- Update `gap.md` and `docs/acceptance/gap-analysis.md` with honest evidence.

## Non-Goals

- No group-scoped audit-log query in this slice.
- No change to audit report authorization behavior.
- No new scheduler, queue, dependency, or storage abstraction.
- No change to retention defaults or environment variable names.
- No live rollout unless the focused and backend test results expose runtime
  risk that needs a live API check.

## Affected Files

- `backend/internal/services/auditcompliance/cleanup.go`
- `backend/internal/services/auditcompliance/cleanup_test.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/handler_test.go`
- `docs/plan/2026-06-21-audit-retention-internal-trigger.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## Verification

```sh
go -C backend test ./internal/services/auditcompliance -run 'AuditRetention|CleanupOldAuditLogs|AuditServiceSpec' -count=1
go -C backend test ./internal/services/auditcompliance
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Implementation Evidence

- Registered `POST /api/v1/internal/audit/cleanup` as a custom handler.
- Added `cleanupAuditRetention`, which reuses `CleanupOldAuditLogs` and reports
  `removed` plus the effective `retention_days`.
- Kept route metadata unchanged: admin, service-internal auth required, and
  policy bypass for internal service traffic only.
- Added ServeHTTP-level tests proving missing/invalid service keys cannot run
  cleanup and a valid service key deletes only expired logs.
- Added default-retention response coverage for unset `AuditRetentionDays`.
- Updated `gap.md` and `docs/acceptance/gap-analysis.md` so audit retention is
  no longer listed as open while group-scoped audit query remains open.
- Verification passed:
  - `go -C backend test ./internal/services/auditcompliance -run 'AuditRetention|CleanupOldAuditLogs|AuditServiceSpec' -count=1`
  - `go -C backend test ./internal/services/auditcompliance`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `git diff --check`

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer reran:
  - `go -C backend test ./internal/services/auditcompliance -run 'AuditRetention|CleanupOldAuditLogs|AuditServiceSpec' -count=1`
  - `git diff --check -- ...slice files...`
- No new dependency, config name, scheduler, storage abstraction, or
  cross-service data ownership was introduced.

## Acceptance

- `POST /api/v1/internal/audit/cleanup` is reachable only through the platform
  service-internal guard.
- A successful internal cleanup call deletes expired audit logs and preserves
  in-retention logs.
- The response reports deterministic retention evidence.
- Audit retention can be marked closed while group-scoped audit query remains
  open in the ledgers.
