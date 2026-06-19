# Audit Event Consumer Contract

## 1. Objective

Add a small, reviewable Day 16-35 contract slice that proves audit-compliance can consume the canonical v1 `AuditEvent` fixture without relying only on legacy outbox payload keys.

## 2. Background

`docs/roadmap.md` requires Day 16-35 event schema fixtures and producer/consumer contract tests, starting with identity, tenant, workload, scheduler, and audit events. `problem.md` says the repo already has canonical v1 event fixtures and several fixture-backed consumer tests, but remaining consumer event paths are still open.

The repo currently has a canonical `AuditEvent` fixture and producer coverage, but audit-compliance reads `AuditEvent` outbox records as an event-fed audit log consumer without fixture-backed consumer coverage.

## 3. Source References

- `AGENTS.md`: three-agent workflow and short feature branch/PR requirements.
- `docs/roadmap.md`: Day 16-35 requires event schema fixtures and producer/consumer contract tests, starting with audit events.
- `problem.md`: remaining consumer event paths are still a High-priority contract testing blocker.
- `backend/internal/contracts/fixtures/events/v1/audit-event.json`: canonical v1 `AuditEvent` payload uses `audit_event_id` and `actor_user_id`.
- `backend/internal/platform/event_producer_contract_test.go`: producer coverage already binds the platform audit hook to the v1 fixture shape while keeping legacy keys.
- `backend/internal/services/auditcompliance/handler.go`: `auditLogs` currently consumes outbox `AuditEvent` payloads using `event.EventID` and legacy `user_id`.
- `backend/internal/services/auditcompliance/workflow_test.go`: existing audit workflow tests publish synthetic legacy `AuditEvent` records.

## 4. Assumptions

- `audit_event_id` is the canonical audit log domain identifier when present; `event.EventID` remains the fallback for legacy events.
- `actor_user_id` is the canonical actor field when present; legacy `user_id`/`userId` remain accepted.
- This slice does not need a new event fixture because `audit-event.json` already exists.
- E2E, live Kubernetes, staging, and deployment evidence are not required because this is an in-process contract and reader compatibility change.

## 5. Non-Goals

- Do not change `/api/v1` route behavior or response schema.
- Do not add database migrations, new infrastructure, or deployment manifests.
- Do not change the `AuditEvent` producer contract or fixture schema.
- Do not implement durable relay, drift comparison, or staging evidence.
- Do not refactor unrelated audit-compliance workflows or project member projections.

## 6. Current Behavior

`auditLogs` builds audit log rows from outbox `AuditEvent` records using `event.EventID` as the log ID and only `user_id` as the actor. A consumer that receives only the canonical v1 fixture payload would preserve action/resource fields but leave the actor blank and use the envelope event ID instead of the canonical audit event ID.

## 7. Target Behavior

`auditLogs` remains backward compatible with legacy events and also accepts canonical v1 keys:

- Log ID prefers payload `audit_event_id`, then `auditEventID`, then `id`, then envelope `event.EventID`.
- Actor ID accepts `user_id`, `userId`, `actor_user_id`, and `actorUserID`.
- Existing action/resource/project/time behavior remains unchanged.
- A new fixture-backed consumer contract test publishes `audit-event.json` into the local event bus and asserts audit-compliance reads it correctly.
- The test also asserts the isolated consumer does not write into the audit owner-store resource.

## 8. Affected Domains

- Audit-compliance event-fed read model behavior.
- Internal event contract testing for the canonical v1 `AuditEvent` fixture.
- GA roadmap blocker tracking in `problem.md`.

## 9. Affected Files

- `backend/internal/services/auditcompliance/handler.go`
- `backend/internal/services/auditcompliance/event_consumer_contract_test.go`
- `problem.md`

## 10. API / Contract Changes

No external `/api/v1` API change.

Internal behavior becomes more tolerant of the existing canonical v1 `AuditEvent` fixture. This is additive and backward compatible for legacy event payloads.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None. The change affects how existing audit events are read, not metrics, logs, or traces.

## 14. Security Considerations

- Do not log or expose secrets.
- Preserve existing auth/admin behavior for audit routes.
- The actor ID is read from already-emitted event payload data; no new trust boundary is introduced.

## 15. Implementation Steps

1. Add focused audit-compliance helper logic in `auditLogs` to resolve canonical and legacy audit IDs and actor IDs.
2. Add `event_consumer_contract_test.go` in audit-compliance that loads `fixtures/events/v1/audit-event.json`, publishes it as a `contracts.Event`, and asserts `auditLogs` plus `RecentAuditLogMaps` preserve canonical fields.
3. Keep the test isolated by verifying the audit owner-store resource remains empty.
4. Update `problem.md` to record the new audit-compliance consumer coverage, local verification, and remaining blockers.
5. Run focused tests and required repository gates.

## 16. Verification Plan

Focused checks:

```bash
go -C backend test ./internal/services/auditcompliance -run 'AuditEvent|Consumer|Contract|RecentAudit' -count=1
go -C backend test ./internal/contracts ./internal/services/auditcompliance -run 'Event|Audit|Consumer|Contract|RecentAudit' -count=1
```

Required gates:

```bash
git diff --check
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
```

Additional security/Sonar evidence should be run when available before merge:

```bash
bash backend/scripts/ci-security-gate.sh security
bash backend/scripts/ci-security-gate.sh sonar
```

E2E, live Kubernetes, and staging evidence are not planned for this slice because no route, deployment, database, or runtime topology behavior changes.

## 17. Rollback Plan

Revert the branch commit. The rollback removes only the audit-compliance compatibility reader change, the fixture-backed consumer test, and the `problem.md` progress note.

## 18. Risks and Tradeoffs

- Risk: preferring `audit_event_id` over the envelope event ID may change internal audit log IDs for new canonical events. Mitigation: this aligns with the fixture's domain ID and falls back to the previous event ID behavior for legacy payloads.
- Risk: adding more tolerated key variants can hide producer drift. Mitigation: the test asserts the canonical v1 keys directly and does not weaken producer fixture coverage.
- Tradeoff: no new generic event mapper is added; keeping the logic local avoids an abstraction for one consumer path.

## 19. Reviewer Checklist

- Requirement fit: Does this close one remaining Day 16-35 audit consumer contract gap?
- Scope control: Are changes limited to audit-compliance, a focused contract test, and `problem.md`?
- Compatibility: Do legacy `user_id`/event ID payloads still work?
- Contract evidence: Does the test load the existing canonical v1 fixture rather than duplicating payload literals?
- Verification: Are focused tests and required gates included?
- Rollback: Can the slice be reverted cleanly without migrations or config cleanup?

## 20. Status

Status: Approved
