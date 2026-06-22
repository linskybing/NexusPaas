# Audit Integrity, RBAC, and Retention Slice

## 1. Objective

Close the remaining local `AUDIT-001..003` launch evidence by hardening audit log
tamper-evidence and adding focused tests for audit query RBAC and retention.

## 2. Background

The audit-compliance service already has admin-only cluster audit logs,
project-scoped report exports, a projected project-member read model, and
retention cleanup. The missing v1 evidence is that audit records are append-only
through normal APIs, query/export paths carry tamper-evident data, and retention
is enforced by a configured cleanup worker.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `backend/internal/services/auditcompliance/spec.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/cleanup.go`
- `backend/internal/services/auditcompliance/workflow_test.go`
- `backend/internal/services/auditcompliance/cleanup_test.go`

## 4. Assumptions

- For v1, "tamper-evident" means query/export results include deterministic
  integrity hashes that change when audit row content or ordering changes. This
  does not replace a future WORM/SIEM backend.
- Normal APIs are the service routes registered in `Spec()` and `Register()`;
  direct database administrator tampering is outside the app API threat model.
- Existing project-member projection is the source of project report RBAC.

## 5. Non-Goals

- No external WORM store, SIEM integration, blockchain/ledger subsystem, or new
  database table.
- No migration of existing audit records.
- No change to who can emit audit events.
- No Web UI or secret API work.

## 6. Current Behavior

- Cluster audit log query is admin-only.
- Project audit report requires authenticated project membership or admin.
- Retention cleanup deletes expired `audit_logs` records based on
  `AUDIT_RETENTION_DAYS`.
- Audit rows do not expose integrity hashes.
- There is no direct test that audit service routes avoid normal update/delete
  APIs.

## 7. Target Behavior

- Every returned audit log carries `integrity_hash` and `previous_hash`.
- Hashes are deterministic over stable audit fields and chained from older to
  newer records.
- CSV exports include integrity columns.
- Tests prove mutating a log changes its integrity hash.
- Tests prove audit log/report resources expose only read routes to users, while
  retention cleanup remains admin plus service-internal.
- Existing RBAC and retention tests remain passing.

## 8. Affected Domains

- Audit-compliance read model and report export.
- Audit-compliance service contract tests.

## 9. Affected Files

- `docs/plan/2026-06-20-audit-integrity-rbac-retention.md`
- `docs/plan/2026-06-20-v1-launch-gap-gate.md`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/workflow_test.go`
- `backend/internal/services/auditcompliance/handler_test.go`

## 10. API / Contract Changes

- `AuditLog` JSON responses add:
  - `integrity_hash`
  - `previous_hash,omitempty`
- CSV report exports add:
  - `Integrity Hash`
  - `Previous Hash`

These are additive response fields.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None. Retention continues using existing `AUDIT_RETENTION_DAYS`.

## 13. Observability Changes

None. Existing request metrics and retention cleanup logs remain unchanged.

## 14. Security Considerations

- Hash-chain evidence detects changes to returned audit rows and ordering.
- The implementation must not hash secrets because audit rows should not contain
  secret values; if future metadata may contain secrets, upstream event
  producers must continue redaction.
- This does not claim protection against privileged database tampering without
  external immutable storage.

## 15. Implementation Steps

- [x] Add deterministic audit-log integrity hash-chain helpers.
- [x] Attach hash-chain fields in the shared audit log reader.
- [x] Include integrity columns in CSV export.
- [x] Add tests for hash-chain mutation detection and route immutability.
- [x] Run focused audit tests, full backend tests, quick gate, and Sonar.
- [x] Update V1 checklist evidence.

## 16. Verification Plan

```sh
go -C backend test ./internal/services/auditcompliance -run 'AuditLog|ProjectReport|Retention|Route' -count=1
go -C backend test ./internal/services -run 'AuditEvents|RouteCoverage|CatalogStateChanging|EventContracts' -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/services/auditcompliance -run 'AuditServiceSpec|Tamper|ProjectReport|AuditLogHandler|Retention' -count=1 -v
go -C backend test ./internal/services/auditcompliance -count=1
go -C backend test ./internal/services -run 'AuditEvents|RouteCoverage|CatalogStateChanging|EventContracts|RegisterAllAdminCoverage' -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed; SonarScanner Quality Gate passed.

## 17. Rollback Plan

Revert this slice. The change is additive response metadata only and has no
schema migration.

## 18. Risks and Tradeoffs

- Consumers that parse CSV by fixed column count must tolerate two new trailing
  columns.
- Hash-chain evidence is local to returned/exported logs. External immutable
  retention can be added later without breaking this contract.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for `AUDIT-001..003` | Pass |
| Scope stays limited to audit read/export evidence | Pass |
| Reuses Go standard crypto/json/csv libraries | Pass |
| SOLID: integrity calculation is isolated from route authorization | Pass |
| 12-Factor: no new hardcoded environment coupling | Pass |
| Security limitations are explicit | Pass |
| Verification plan is concrete | Pass |
| Rollback is realistic | Pass |
| Simplicity / no over-engineering | Pass |

## 20. Status

Status: Implemented and reviewer-verified for this slice.

Reviewer Agent: Approved and verified. The implementation adds deterministic
audit hash-chain evidence, preserves audit query/report RBAC, verifies
append-only route shape through service spec tests, retains configured retention
cleanup, and passes focused, full, quick, and Sonar verification. External WORM
or SIEM storage remains a post-v1 hardening option, not a local v1 blocker.
