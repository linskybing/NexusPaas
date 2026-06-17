# Current Backend Function Inventory

## 1. Objective

Create `/Users/sky/workspaces/function.md` as the single current-backend
capability inventory for NexusPaas Production Beta planning. The document must
map implemented and documented capabilities to the existing 15-service catalog,
show route/event/background-worker coverage, and identify ownership,
dependencies, and unresolved parity risks.

## 2. Background

`problem.md` currently lists missing `function.md` as a High blocker because
there is no single capability inventory for launch readiness and parity
planning. The previous plan assumed the legacy reference snapshot at
`references/CSCC_AI_Platform_Backend` was present, but the current repository
state does not contain that snapshot. This PR must therefore not claim live
reference parity. It will close only the missing-inventory blocker by creating a
current-backend source of truth, while leaving the missing reference snapshot as
a separate launch risk.

## 3. Source References

- `problem.md`
- `backend/README.md`
- `backend/docs/api-route-mapping.md`
- `backend/docs/event-contracts.md`
- `backend/docs/migration-roadmap.md`
- `backend/docs/non-functional-requirements.md`
- `backend/docs/operational-readiness.md`
- `backend/docs/beta-launch-readiness.md`
- `backend/internal/services/catalog.go`
- `backend/*-service/README.md`
- `backend/platform-gateway/README.md`

## 4. Assumptions

- `references/CSCC_AI_Platform_Backend` is unavailable in the current worktree.
- `function.md` should be created at the repository root.
- The inventory uses the existing 15-service catalog and does not introduce new
  service boundaries.
- This is a documentation and launch-readiness bookkeeping change only.
- Existing local `long-term.md` is user-owned and remains untracked.

## 5. Non-Goals

- Do not implement, refactor, or re-route services.
- Do not modify migrations, Kubernetes manifests, CI scripts, runtime config, or
  production code.
- Do not claim reference parity without the missing reference snapshot.
- Do not invent additional services or split by controller/table.
- Do not remove live staging, observability provisioning, coverage, or data
  boundary risks from `problem.md`.

## 6. Current Behavior

The repository has route mapping, service README files, event contracts,
operational readiness docs, and the service catalog, but there is no single
`function.md` that gives reviewers and launch owners a current capability
inventory. `problem.md` therefore still reports missing capability inventory as
a High blocker.

## 7. Target Behavior

`function.md` provides a concise but complete current-backend inventory. It
must:

- list all 15 target services,
- group user-visible and operational capabilities by domain,
- map each capability to target service, route/job/event evidence, owned data,
  dependencies, and notes,
- separately list non-HTTP background jobs and maintenance workers,
- identify gateway/security/audit obligations,
- state that reference parity is still unverified because the reference snapshot
  is absent.

`problem.md` should then downgrade the `function.md` blocker to resolved
evidence while preserving the separate reference snapshot blocker.

## 8. Affected Domains

- Platform gateway and edge routing
- Identity, account management, OIDC, and user API tokens
- Authorization policy, domain RBAC, proxy RBAC, and policy sync
- Organization, groups, projects, membership, and tenancy
- Workload config, jobs, scheduling, quota, and Kubernetes control
- IDE workspaces, storage, image registry, media upload, integrations
- Usage/dashboard, audit/compliance, requests, notifications, announcements
- Production Beta release governance and problem tracking

## 9. Affected Files

- Update `docs/plan/2026-06-16-reference-backend-function-inventory.md`
- Add `function.md`
- Update `problem.md`

## 10. API / Contract Changes

None. Existing external `/api/v1` routes, internal owner-read contracts,
background workers, events, and response contracts are not changed. The
inventory documents current contracts only.

## 11. Database / Migration Changes

None. The inventory names owned data areas but does not change schemas,
migrations, ownership, or storage behavior.

## 12. Configuration Changes

None. The inventory can mention operational dependencies and configuration
surfaces, but no environment variables, ConfigMaps, Secrets, or deployment
files change.

## 13. Observability Changes

No runtime telemetry changes. The inventory documents existing observability,
audit, usage, metrics, dashboard, and synthetic-smoke expectations so launch
review can trace capabilities to operational evidence.

## 14. Security Considerations

- Preserve the distinction between gateway auth, service-to-service owner reads,
  JWT-only browser/proxy routes, OIDC, API tokens, and proxy RBAC.
- Mark admin, permission-changing, tenant-changing, job/storage/image, and
  integration operations as audit-relevant where applicable.
- Do not add credentials, tokens, secret values, or example production secrets.
- Do not weaken `problem.md` security or live staging blockers.

## 15. Implementation Steps

1. Revise this plan to reflect the current repository state and the missing
   reference snapshot.
2. Obtain Reviewer Agent approval before creating `function.md`.
3. Add `function.md` with:
   - launch-readiness scope and evidence limits,
   - 15-service catalog summary,
   - capability inventory table,
   - non-HTTP/background-worker checklist,
   - cross-service contract and audit/security notes,
   - remaining gaps.
4. Update `problem.md`:
   - mark capability inventory as resolved with `function.md` evidence,
   - keep missing reference snapshot as a High launch blocker,
   - keep other blockers unchanged unless wording must reference the new file.
5. Run focused verification.
6. Submit implementation to Reviewer Agent and fix any requested changes.

## 16. Verification Plan

- `git diff --check`
- `test -f function.md`
- `for service in platform-gateway identity-service authorization-policy-service org-project-service workload-service scheduler-quota-service k8s-control-service ide-service storage-service image-registry-service usage-observability-service audit-compliance-service request-notification-service integration-proxy-service media-upload-service; do rg -q "$service" function.md || exit 1; done`
- `for worker in "audit cleanup" "Harbor health" "LDAP mirror" "cluster resource collector" "GPU usage collector" "resource hours collector" "resource quota reconciler" "priority class sync" "idle reaper" "plan window reaper" "workload runtime reaper" "policy data sync" "Longhorn RWX" "VPN usage collector" "queue metrics collector" "job dispatcher"; do rg -qi "$worker" function.md || exit 1; done`
- `rg -n "reference parity.*unverified|reference snapshot.*absent|references/CSCC_AI_Platform_Backend.*absent" function.md`
- `rg -n "reference parity.*unverified|reference snapshot.*absent|references/CSCC_AI_Platform_Backend.*absent" problem.md`
- `rg -n "\\| High \\| reference parity \\| .*references/CSCC_AI_Platform_Backend.*absent" problem.md`
- `cd backend && go test ./internal/platform ./internal/services -count=1`
- `cd backend && go vet ./...`
- `cd backend && go build ./...`

Full Docker-backed, security, and Sonar gates are not required for this
documentation-only slice unless reviewer requests them; the previous `main`
non-live RC evidence remains valid but will not be reused as proof for this PR's
new file content.

## 17. Rollback Plan

Rollback is limited to reverting this documentation PR:

- remove `function.md`,
- restore the previous plan wording,
- restore the previous `problem.md` missing-inventory entry.

No runtime, data, migration, or deployment rollback is needed.

## 18. Risks and Tradeoffs

- Risk: Readers may mistake current-backend inventory for reference parity.
  Mitigation: `function.md` and `problem.md` explicitly state reference parity is
  unverified until the missing snapshot is restored.
- Risk: A broad table becomes too large to maintain. Mitigation: group related
  endpoints by capability and rely on route/service docs for exhaustive route
  details.
- Risk: Documentation drift. Mitigation: cite current route mapping, service
  catalog, event contracts, and operational readiness docs as the evidence base.
- Tradeoff: This PR closes the inventory blocker but does not resolve live
  staging or reference parity.

## 19. Reviewer Checklist

| Category | Check |
| --- | --- |
| Requirement Fit | `function.md` exists and gives a current-backend capability inventory. |
| Scope Control | Only plan/docs/problem tracking files change. |
| Architecture | Inventory follows the existing 15-service bounded-context catalog. |
| Microservice Boundary | No new service boundaries are introduced. |
| API Contract | Routes/contracts are documented but not changed. |
| Data Ownership | Each capability group identifies owned data at service level. |
| Config | No runtime configuration changes are made. |
| Observability | Background jobs, usage, metrics, audit, and synthetic smoke are represented. |
| Security | Gateway auth, JWT-only routes, proxy RBAC, service auth, and audit obligations are preserved. |
| Testing | Verification checks file existence, service/background coverage, diff cleanliness, and relevant Go packages. |
| Rollback | Reverting documentation files is sufficient. |
| Diff Scope | No unrelated files or untracked `long-term.md` are included. |

## 20. Status

Status: Approved
