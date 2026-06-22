# Audit Project-Scoped Query RBAC

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Reduce the `AUDIT-001..004` gap by making `/api/v1/audit/logs` enforce
domain-scoped audit query access instead of platform-admin-only access.

## Current Evidence

- `gap.md` marks audit as partial because RBAC-scoped query and enforced
  retention remain open.
- Before this slice, `Spec()` marked `GET /api/v1/audit/logs` with `admin()`,
  so project-scoped readers were rejected before handler-level domain checks.
- `downloadProjectReport` already uses the audit service's event-fed project
  member read model, but `getAuditLogs` does not.

## Scope

- Remove the route-level platform admin gate from `GET /api/v1/audit/logs` so
  handler-level domain checks can run.
- Keep the route authenticated and PDP-controlled; do not add a policy bypass.
  This slice changes handler-level authorization only after middleware/PDP has
  already allowed the request.
- Add route-level tests proving `GET /api/v1/audit/logs` has no route `Admin`,
  has no `PolicyBypass`, and PDP denial still blocks access before the handler.
- Allow platform admin and exact `platform_auditor` role to query all audit
  logs. Do not accept fuzzy auditor/admin spellings beyond the existing platform
  admin check.
- For non-platform readers, require `project_id`.
- Allow only that project's admin role to query that project's logs.
- Filter non-platform responses to the requested `project_id`, regardless of
  other query filters.
- Reuse the existing project member projection/read model; do not add a new
  read model or cross-service repository access.
- Support the local role spellings already used by the codebase (`admin`) and
  the GA role spelling (`project_admin`) for project membership records.
- Use a separate audit-log query helper so `/api/v1/audit/report` keeps its
  current project-member behavior.
- Add focused audit-compliance tests for:
  - platform admin can query all logs,
  - exact `platform_auditor` can query all logs but without route admin
    metadata,
  - ordinary user without `project_id` is rejected,
  - project admin with membership role `admin` sees only their project's audit
    logs,
  - project admin with membership role `project_admin` sees only their project's
    audit logs,
  - project non-admin member cannot query project audit logs.
  - `/api/v1/audit/report` still allows ordinary project members as before.
- Update `gap.md` and this plan with evidence.

## Non-Goals

- No group-scoped audit query in this slice.
- No retention enforcement in this slice.
- No PDP policy bootstrap for platform auditor in this slice.
- No live rollout unless local checks expose runtime risk that needs live
  evidence.

## Affected Files

- `backend/internal/services/auditcompliance/spec.go`
- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/handler_test.go`
- `docs/plan/2026-06-21-audit-project-scoped-query-rbac.md`
- `gap.md`

## Verification

```sh
go -C backend test ./internal/services/auditcompliance -run 'Audit.*Query|AuditServiceSpec' -count=1
go -C backend test ./internal/services/auditcompliance
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Implementation Evidence

- Removed route-level `admin()` from `GET /api/v1/audit/logs` while preserving
  authentication and PDP enforcement.
- Added handler-level audit query authorization:
  - platform admin can query all logs,
  - exact `platform_auditor` can query all logs,
  - non-platform readers must provide `project_id`,
  - only project membership roles `admin` and `project_admin` can query that
    project's logs.
- Added project-id filtering to audit log query parameters.
- Kept `/api/v1/audit/report` on the existing project-member access behavior.
- Added route/middleware tests proving audit logs route has no route `Admin`,
  has no `PolicyBypass`, and PDP denial still blocks access before handler
  authorization.
- Verification passed:
  - `go -C backend test ./internal/services/auditcompliance -run 'Audit.*Query|AuditServiceSpec|AuditLogsRoute|ProjectReportDownload' -count=1`
  - `go -C backend test ./internal/services/auditcompliance`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `git diff --check`

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer reran:
  - `go -C backend test ./internal/services/auditcompliance -run 'Audit.*Query|AuditServiceSpec|AuditLogsRoute|ProjectReportDownload' -count=1`
  - `go -C backend test ./internal/services/auditcompliance -count=1`
- Non-blocking residual risk: live reachability for project admins and
  `platform_auditor` still depends on PDP policy data allowing the request
  before handler RBAC, which is consistent with this slice's no-bootstrap scope.

## Acceptance

- Audit log query is no longer platform-admin-only.
- Project admin can query only their project's audit logs.
- Project non-admin member cannot query project audit logs.
- Platform auditor/admin all-log access is represented in handler tests and
  route metadata no longer blocks it.
- `gap.md` remains honest that group-scoped audit query and retention are still
  open.
