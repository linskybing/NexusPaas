# audit-compliance-service

Category: Ops | Phase: 1 (first batch)

## 1. Overview

The audit and compliance service. Responsible for audit event ingestion, audit logs, project audit reports, security posture, and retention/cleanup. Low coupling and high value, using an async event bus (outbox, at-least-once delivery) — recommended for the first extraction batch.

## 2. Functional Requirements

| ID | Requirement | Acceptance/Notes |
| --- | --- | --- |
| FR-OBS-04 | Record audit logs including user, time, action, resource, success/failure, and IP. | Async writes via the audit bus. |
| FR-OBS-05 | Support project audit report download and admin security posture queries. | /audit/report, /admin/security/posture. |

## 3. Owned Data

`audit_logs`, security findings/reports, event ingestion offsets.

## 4. Current Code/Route Mapping

- Handlers: `audit.go`
- Application: `application/audit`
- Docs: ops audit docs
- Routes: `/api/v1/audit/*`, `/api/v1/admin/security/posture`

## 5. Events

| Direction | Event | Source | Purpose |
| --- | --- | --- | --- |
| Subscribe | AuditEvent | **all services** | Published via outbox with at-least-once delivery; this service deduplicates idempotently and persists |
| Subscribe | PolicyChanged, important Job/Storage/Image state events | various services | Compliance tracking |

## 6. Required AuditEvent Fields

- `event_id` (idempotent dedup), `occurred_at`, `trace_id`
- User (user_id), action, resource (type + id), success/failure, source IP
- Domain context (project_id/group_id where applicable)

## 7. Non-Functional Highlights

- All administrative operations, permission changes, and important Job/Storage/Image state changes MUST produce audit events (NFR-SEC-05) — this service is the mandatory sink.
- Audit ingestion workers need horizontal sharding (NFR-SCALE-02); audit backlog is a core monitoring metric (NFR-OBS-02).
- Retention/cleanup is configuration-driven (NFR-OPER-02).
- Every authorization decision must be traceable: token validation, RBAC decisions, proxy policies, domain extraction all have audit or debug traces (acceptance criterion).

## 8. Decomposition Notes

Phase 1, first batch. Extracting it also introduces the outbox/inbox and AuditEvent contract into the monolith, serving as the event infrastructure template for all subsequent services.
