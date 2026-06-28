# Internal Command Contracts

Internal command contracts are service-key-gated HTTP writes used while the backend is decomposed into deployable units. They document existing owner-side state changes and consumer client expectations without changing external `/api/v1` compatibility.

## Scheduler Compute Fixture Catalog

Canonical v1 command fixtures live under `backend/internal/contracts/fixtures/commands/v1/` and are validated by `backend/internal/contracts` plus focused owner/consumer service tests.

| Command | Fixture | Owner | Consumer | Method | Path | Idempotency | Emitted Events |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Bind project plan | `org-project-bind-project-plan.json` | `org-project-service` | `scheduler-quota-service` | `PUT` | `/internal/org-project/projects/{project_id}/plan` | Same `project_id` and `plan_id` can be retried safely. | `ProjectUpdated` |
| Clear plan bindings | `org-project-clear-plan-bindings.json` | `org-project-service` | `scheduler-quota-service` | `DELETE` | `/internal/org-project/plans/{plan_id}/project-bindings` | Same `plan_id` can be retried; zero cleared is a valid success. | `ProjectUpdated` when bindings change |
| Preempt workload job | `workload-preempt-job.json` | `workload-service` | `scheduler-quota-service` | `POST` | `/internal/workload/jobs/{id}/preempt` | Same `id` and `preemption_id` can be retried and returns the existing job record. | None in current handler |
| Evict workload job | `workload-evict-job.json` | `workload-service` | `scheduler-quota-service` | `POST` | `/internal/workload/jobs/{id}/evict` | Same `id` can be retried after eviction and returns the existing job record. | None in current handler |
| Dispatch FastTransfer mover | `k8s-control-dispatch-fast-transfer-mover.json` | `k8s-control-service` | `storage-service` | `POST` | `/internal/k8s-control/fast-transfers/mover-jobs` | Same `target_namespace`, `name`, and `transfer_id` can be retried and returns `already_exists`. | None |

## Contract Rules

- Fixtures use `schema_version: 1` and must remain additive-only. Consumers must tolerate unknown top-level, request, and response fields.
- Every fixture declares `auth: service_key` and `service_key_required: true`; user auth bypass in route metadata does not replace handler-level service-key checks.
- Command fixtures describe owner-write boundaries only. Project plan binding writes remain inside `org-project-service`; job preemption and eviction writes remain inside `workload-service`; Kubernetes mover Job creation remains inside `k8s-control-service`.
- Fixture examples are synthetic and must not include raw database primary keys, internal row IDs, secrets, tokens, passwords, cookies, credentials, owner passwords, connector auth, tunnel tokens, or local metadata.
- External `/api/v1` routes and response envelopes are unchanged by these artifacts.
- Breaking command changes require a new versioned fixture or compatibility migration plan before a deployable-unit split relies on the contract.

## Follow-Up Work

- Add broader command fixtures for other state-changing internal boundaries.
- Add producer-specific event fixtures and consumer contract tests for the events emitted by these owner writes.
- Add Outbox/Inbox persistence, idempotent consumers, retry/dead-letter visibility, and lag metrics.
- Replace high-churn owner-read paths with event-fed read models where the Day 36-55 roadmap requires it.
