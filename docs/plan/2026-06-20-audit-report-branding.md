# Audit Report Branding Slice

## 1. Objective

Close the smallest launch-blocking `AUDIT-*` gap by making exported audit
reports use the configured product brand instead of a hardcoded generic
filename/content identity.

## 2. Background

`docs/acceptance/naming.md` requires exported user audit reports to use the
configured brand naming. The audit-compliance service already exposes admin-only
cluster audit logs, project-scoped report downloads, and retention cleanup. The
current project report export is CSV but uses `audit_report_<project>.csv`
without product-brand evidence.

## 3. Source References

- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/naming.md`
- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/workflow_test.go`

## 4. Assumptions

- `PRODUCT_NAME` is the smallest useful brand configuration for this backend
  slice. Broader brand tokens such as CLI binary, domains, labels, namespace,
  and metric prefix remain separate acceptance work.
- Default product name remains `NexusPaaS`.
- Report export evidence can be satisfied by branded CSV metadata and a branded
  attachment filename.

## 5. Non-Goals

- No audit storage redesign, WORM backend, external SIEM integration, or new
  event broker.
- No Web UI work.
- No broad rebrand implementation across CLI, labels, domains, OpenAPI, or
  docs.
- No database migration.

## 6. Current Behavior

- Project audit report download requires an authenticated user and project
  membership or admin.
- The CSV contains matching project rows only.
- The attachment filename is `audit_report_<project>.csv`.
- There is no product-name config in `platform.Config`.

## 7. Target Behavior

- `ConfigFromEnv` loads `PRODUCT_NAME` with default `NexusPaaS`.
- Runtime helpers treat empty product names as the safe default.
- Project audit report CSV includes product metadata.
- The CSV attachment filename is prefixed by a safe slug derived from the
  configured product name.
- Existing authorization, filtering, and CSV row behavior remain unchanged.

## 8. Affected Domains

- Platform runtime configuration.
- Audit-compliance report export.
- Backend unit tests and V1 checklist documentation.

## 9. Affected Files

- `docs/plan/2026-06-20-audit-report-branding.md`
- `docs/plan/2026-06-20-v1-launch-gap-gate.md`
- `backend/internal/platform/config.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/workflow_test.go`

## 10. API / Contract Changes

- `GET /api/v1/audit/report` still returns `text/csv`.
- The `Content-Disposition` filename changes from
  `audit_report_<project>.csv` to
  `<product-slug>_audit_report_<project>.csv`.
- CSV output gains a small metadata preamble with `Product` and `Project`
  before the existing audit row header.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Add:

- `PRODUCT_NAME`: user-facing product title for exported reports. Default:
  `NexusPaaS`.

## 13. Observability Changes

None. The report path remains covered by existing request metrics and audit
events.

## 14. Security Considerations

- Brand names are sanitized before use in attachment filenames.
- CSV cells are emitted through Go's standard `encoding/csv` writer.
- The change does not alter audit read authorization or project filtering.
- No secrets or credentials are added to report metadata.

## 15. Implementation Steps

- [x] Add product-name config field, env constant, default, and effective helper.
- [x] Add config parsing/default tests.
- [x] Update audit report export to include branded filename and CSV metadata.
- [x] Add report export tests for configured brand and existing project filter.
- [x] Run focused audit/platform tests, full backend tests, quick gate, and
  Sonar.
- [x] Update V1 checklist evidence.

## 16. Verification Plan

```sh
go -C backend test ./internal/platform -run 'ProductName|Config' -count=1
go -C backend test ./internal/services/auditcompliance -run 'ProjectReport' -count=1
go -C backend test ./internal/services/auditcompliance -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/platform -run 'ProductName|ConfigInputLimit|ConfigServiceEnvDefaults' -count=1 -v
go -C backend test ./internal/services/auditcompliance -run 'ProjectReport|AuditLogHandler' -count=1 -v
go -C backend test ./internal/services/auditcompliance -count=1
go -C backend test ./internal/platform -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed; SonarScanner Quality Gate passed.

## 17. Rollback Plan

Revert this slice. No persistent schema or external service state changes are
involved.

## 18. Risks and Tradeoffs

- Adding a CSV metadata preamble may require consumers that assumed the first
  row was the audit header to skip metadata rows. This is acceptable for a v1
  launch report because it makes the configured product identity explicit.
- This does not complete the entire naming policy; it only closes the audit
  report export evidence required for this gap.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit for `AUDIT-004` / `NAME-08` | Pass |
| Scope stays limited to report export and config | Pass |
| Reuses standard library CSV and existing config path | Pass |
| SOLID: config, filename sanitization, and report writing stay isolated | Pass |
| 12-Factor: product name is externalized config with safe default | Pass |
| Security: filename is sanitized and authorization is unchanged | Pass |
| Verification plan is concrete | Pass |
| Rollback is realistic | Pass |
| Simplicity / no over-engineering | Pass |

## 20. Status

Status: Implemented and reviewer-verified for this slice.

Reviewer Agent: Approved and verified. The implementation directly addresses
`AUDIT-004` / `NAME-08` with a small, reversible change, keeps report
authorization unchanged, externalizes the product title through `PRODUCT_NAME`,
and passes focused, full, quick, and Sonar verification.
