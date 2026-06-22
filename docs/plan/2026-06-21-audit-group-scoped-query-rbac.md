# Audit Group-Scoped Query RBAC

Status: Implemented; Reviewer approved

Reviewer: Final implementation review approved

## Objective

Close the remaining `AUDIT-001..004` audit-query gap by allowing group admins to
query only their group's audit logs while preserving platform auditor/admin and
project-scoped behavior.

## Current Evidence

- `gap.md` now marks `AUDIT-001..004` partial only because group-scoped query is
  still open.
- `GET /api/v1/audit/logs` already runs after normal authentication and PDP, and
  handler-level RBAC now supports platform admin, exact `platform_auditor`, and
  project-admin scoped queries.
- Audit records can already carry `group_id` because platform audit publishing
  writes `group_id` from the request query, but `AuditLog`, parsing, filtering,
  and handler authorization do not expose/use it.
- Audit logs are assembled from both persisted `audit_logs` records and
  `AuditEvent` entries in `app.Events.Outbox()`, and dashboard-style readers use
  `RecentAuditLogMaps`; all of those paths must preserve `group_id`.
- Audit-compliance already has an event-fed project member read model. It does
  not yet have a group membership read model and must not directly depend on the
  org-project owner store in isolated mode.
- `org-project-service` emits `GroupMembershipChanged` events for create,
  update, and delete membership paths.

## Scope

- Add `GroupID` to `AuditLog`, `logFromMap`, outbox-sourced `AuditEvent`
  mapping in `auditLogs()`, integrity hash payload, `RecentAuditLogMaps`, and
  audit query filtering.
- Parse `group_id` in audit-log query parameters.
- Update non-platform audit query authorization to accept either:
  - project admin access for `project_id`, or
  - group admin access for `group_id`.
- If neither `project_id` nor `group_id` is supplied by a non-platform reader,
  return `400` with a scoped-query-required message.
- Treat group membership roles `admin` and `group_admin` as group-admin roles.
- Add an audit-compliance owned group membership read model fed by
  `GroupMembershipChanged` events:
  - create/action payloads create or upsert local membership rows,
  - `new` update payloads update local membership rows,
  - delete/action payloads delete local membership rows.
- Mirror the existing project-member projection behavior:
  - run through `app.RunProjection`,
  - use local audit-compliance resources in isolated mode,
  - merge source rows only when `org-project-service` is co-hosted,
  - write dead letters on projection write failure through the platform
    projection runner.
- Declare the audit-owned group membership read model in `Spec().Tables` and add
  a focused spec/isolation assertion so service-owned data is visible in the
  catalog.
- Add focused tests for:
  - group admin can query only logs for their group,
  - `group_admin` role spelling is accepted,
  - ordinary group member cannot query group audit logs,
  - non-platform reader without `project_id` or `group_id` gets `400`,
  - `group_id` filtering works for outbox-sourced `AuditEvent` logs and
    `RecentAuditLogMaps` preserves `group_id`,
  - group membership create/update/delete events maintain the read model,
  - group membership projection write failure creates a dead letter,
  - isolated audit-compliance does not read the owner `user_groups` resource.
- Keep `/api/v1/audit/report` behavior unchanged.
- Update `gap.md`, `docs/acceptance/gap-analysis.md`, and this plan with
  evidence.

## Non-Goals

- No retention changes.
- No PDP policy bootstrap changes.
- No changes to org-project service membership APIs.
- No new cross-service synchronous read path.
- No new dependency or schema migration.
- No live rollout unless local checks expose runtime risk that needs live API
  evidence.

## Affected Files

- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/handler_test.go`
- `backend/internal/services/auditcompliance/spec.go`
- `backend/internal/services/auditcompliance/workflow_test.go`
- `backend/internal/services/service_dependency_inventory_test.go`
- `docs/plan/2026-06-21-audit-group-scoped-query-rbac.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## Verification

```sh
go -C backend test ./internal/services/auditcompliance -run 'Audit.*Query|GroupMemberProjection|AuditServiceSpec|ProjectReportDownload' -count=1
go -C backend test ./internal/services/auditcompliance -run 'CanQueryGroupUsesEventFedGroupMembersWhenIsolated' -count=1
go -C backend test ./internal/services/auditcompliance
go -C backend test ./...
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

## Implementation Evidence

- Added `group_id` support to `AuditLog`, stored-row mapping, outbox-sourced
  `AuditEvent` mapping, integrity hash payloads, `RecentAuditLogMaps`, query
  parsing, and query filtering.
- Added handler-level group-scoped audit query authorization:
  - platform admin and exact `platform_auditor` behavior remains unchanged,
  - project-admin scoped query remains unchanged,
  - group roles `admin` and `group_admin` can query matching `group_id` logs,
  - ordinary group members and unscoped non-platform readers are rejected.
- Added an audit-owned `group_report_members` read model declared in
  `Spec().Tables`, fed by `GroupMembershipChanged` events.
- Kept isolated service boundaries: audit-compliance only uses local projected
  group membership rows unless `org-project-service` is co-hosted; the
  co-hosted fallback is explicitly classified in the service dependency
  inventory.
- Added tests for group query RBAC, outbox-sourced `group_id` filtering,
  `RecentAuditLogMaps` `group_id` preservation, group projection
  create/update/delete, projection write-failure dead lettering, and isolated
  owner-store avoidance.
- Updated `gap.md` and `docs/acceptance/gap-analysis.md` so `AUDIT-001..004` is
  now Done while broader GA blockers remain open.
- Verification passed:
  - `go -C backend test ./internal/services/auditcompliance -run 'Audit.*Query|GroupMemberProjection|AuditServiceSpec|ProjectReportDownload' -count=1`
  - `go -C backend test ./internal/services/auditcompliance -run 'CanQueryGroupUsesEventFedGroupMembersWhenIsolated' -count=1`
  - `go -C backend test ./internal/services/auditcompliance`
  - `go -C backend test ./...`
  - `bash backend/scripts/ci-security-gate.sh quick`
  - `git diff --check`

## Reviewer Result

- Reviewer found no blocking findings and approved final implementation review.
- Reviewer reran:
  - `go -C backend test ./internal/services/auditcompliance -run 'Audit.*Query|GroupMemberProjection|AuditServiceSpec|ProjectReportDownload' -count=1`
  - `go -C backend test ./internal/services/auditcompliance -run 'CanQueryGroupUsesEventFedGroupMembersWhenIsolated' -count=1`
  - `go -C backend test ./internal/services -run 'ServiceDependency|ServiceIsolation|AuditComplianceWorkflow' -count=1`
  - `git diff --check -- ...slice files...`
- Reviewer confirmed the event-fed read-model boundary, service catalog table,
  co-hosted fallback classification, and ledger update are acceptable.

## Acceptance

- Platform admin and exact `platform_auditor` all-log access still works.
- Project-scoped audit query behavior remains unchanged.
- Group admins can query only logs matching their `group_id`.
- Ordinary group members and unrelated users cannot query group audit logs.
- `AUDIT-001..004` can move from partial to done in the ledgers if this slice
  passes review, because query RBAC, retention, integrity, and brand evidence
  are all present.
