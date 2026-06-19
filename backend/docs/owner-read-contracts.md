# Owner-Read Contracts

Owner-read contracts are service-key-gated internal HTTP reads used when a consumer needs an owning service snapshot before an event-fed read model exists. They are transitional contracts for decomposition: owners keep write authority, consumers read through explicit paths, and follow-up slices should replace high-churn reads with read models where appropriate.

## Scheduler Admission Fixture Catalog

Canonical v1 owner-read fixtures for the scheduler admission boundary live under `backend/internal/contracts/fixtures/owner-read/v1/` and are validated by `backend/internal/contracts` and `backend/internal/platform` tests.

| Resource | Fixture | Owner | Consumer | List Path | Get Path | Semantics |
| --- | --- | --- | --- | --- | --- | --- |
| `org-project-service:projects` | `org-project-projects.json` | `org-project-service` | `scheduler-quota-service` | `/internal/org-project/projects` | `/internal/org-project/projects/{id}` | Project snapshot used to resolve plan, owner, and default queue metadata. |
| `org-project-service:project_members` | `org-project-project-members.json` | `org-project-service` | `scheduler-quota-service` | `/internal/org-project/project-members` | `/internal/org-project/project-members/{id}` | Direct membership snapshot keyed by `project_id/user_id`. |
| `org-project-service:user_quotas` | `org-project-user-quotas.json` | `org-project-service` | `scheduler-quota-service` | `/internal/org-project/user-quotas` | `/internal/org-project/user-quotas/{id}` | User quota snapshot keyed by `project_id/user_id`. |
| `org-project-service:user_groups` | `org-project-user-groups.json` | `org-project-service` | `scheduler-quota-service` | `/internal/org-project/user-groups` | `/internal/org-project/user-groups/{id}` | Group membership snapshots used for group-owned project access checks. |
| `workload-service:jobs` | `workload-jobs.json` | `workload-service` | `scheduler-quota-service` | `/internal/workload/jobs` | n/a | List-only workload usage snapshot for active job aggregation. |

## Contract Rules

- Fixtures use `schema_version: 1` and must remain additive-only. Consumers tolerate unknown top-level fields and additive record `data` fields.
- Every fixture declares `auth: service_key` and `service_key_required: true`; internal owner-read endpoints must reject missing or wrong service credentials.
- Fixture records decode as `contracts.Record[map[string]any]` with stable `id`, `version`, `created_at`, `updated_at`, and `data` fields.
- Fixture payloads must not include raw database primary keys, internal row IDs, secrets, tokens, passwords, cookies, credentials, owner passwords, connector auth, tunnel tokens, or local metadata.
- `workload-service:jobs` is intentionally list-only. Scheduler admission uses it for aggregate running usage and must not assume a job get contract exists.
- These fixtures preserve existing `/api/v1` external compatibility; they document internal owner-read contracts only.

## Follow-Up Work

- Add command API fixtures for cross-service write-like internal calls.
- Add producer-specific event fixtures and consumer contract tests beyond the core envelope slice.
- Add Outbox/Inbox persistence, idempotent consumers, retry/dead-letter visibility, and lag metrics.
- Replace high-churn owner-read paths with owner-fed read models where the Day 36-55 roadmap requires it.
