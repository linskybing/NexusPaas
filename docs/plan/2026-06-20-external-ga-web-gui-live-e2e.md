# External GA Web GUI And Live E2E Roadmap

## 1. Objective

Track NexusPaaS External GA blockers and split them into reviewed, testable
slices. This file is a roadmap, not approval to implement all GA work in one
diff. Each code change still needs its own small approved plan.

## 2. Background

The current backend is a Go modular monolith with service-boundary awareness and
an 8-unit Production Beta runtime target. Local beta gates have passed, but
External GA still has open P0/P1 blockers, no frontend app, and no live-like
staging evidence bundle for every required GA surface.

## 3. Source References

- `problem.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/ga-checklist.md`
- `docs/acceptance/iteration-plan.md`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`
- `docs/adr/0002-outbox-inbox-read-models.md`
- `docs/adr/0004-deployment-evidence-gates.md`

## 4. Assumptions

- "正式版" means External GA, not Production Beta.
- `gap.md` means `docs/acceptance/gap-analysis.md`; no root `gap.md` exists.
- WebRPC GUI means a first-party management Web GUI. GA v1 uses existing
  REST/OpenAPI instead of adding a new typed RPC layer.
- Staging/live credentials and external infrastructure may be unavailable in a
  local agent session; missing evidence remains tracked as a blocker.

## 5. Non-Goals

- No big-bang rewrite into 15 physical microservices.
- No new RPC layer before an existing API gap proves it is needed.
- No custom identity, policy, registry, metrics, or secret platform when a
  CNCF/OSS integration already fits.
- No emptying blocker docs without code, test, review, and live evidence.

## 6. Current Behavior

`problem.md` still lists P0/P1/P2 blockers. `docs/acceptance/gap-analysis.md`
still marks WEB as conditional and lists launch gaps. The repo has no existing
frontend package. Live evidence currently proves a local RKE2 beta-style rollout,
not a full External GA staging promotion.

## 7. Target Behavior

External GA is declared only after:

- all P0 blockers are closed;
- WEB acceptance is implemented and tested because GA now includes Web GUI;
- local, CI, and live-like E2E gates pass;
- rollback, redeploy, backup/restore, and failure-injection evidence exists;
- `problem.md` and gap analysis contain no unresolved GA blocker.

## 8. Affected Domains

- Platform runtime, routing, auth, policy, and API-token verification.
- Identity, project, workload, scheduler/quota, storage, registry, usage, and
  audit ownership.
- Frontend management GUI and browser E2E.
- Release evidence, checklists, and reviewer gates.

## 9. Affected Files

This roadmap does not approve product-code edits. Each slice plan must list its
precise file set before coding.

## 10. API / Contract Changes

- Existing REST/OpenAPI APIs remain the GUI backend contract.
- Any new GUI-required endpoint must be added to OpenAPI and tested through the
  same auth/RBAC policy path as CLI/API.
- OIDC login must be the Web GUI login path; manual token handling is not an
  acceptable GA Web flow.

## 11. Database / Migration Changes

Future slices may add typed domain tables, transactional outbox/inbox tables,
inbox idempotency records, retry/dead-letter state, and migration metadata. Each
schema slice must include migration tests, rollback notes, and drift/replay
evidence.

## 12. Configuration Changes

Future slices must externalize configuration through environment variables or
Kubernetes-managed config. GA staging/production must fail closed without a real
PDP, safe service identity, trusted proxy config, OIDC issuer/client settings,
and required backing services.

## 13. Observability Changes

Release evidence must include request IDs, trace IDs, image digests, route
smoke results, metrics availability, outbox/inbox lag where relevant, audit
events, and GUI E2E artifacts.

## 14. Security Considerations

- Do not log or expose secrets, tokens, or raw credentials through API, CLI, GUI,
  audit export, or test artifacts.
- Web GUI must enforce the same RBAC as APIs.
- Token verification must become indexed by token id/prefix plus one hash check.
- Trusted client IP resolution must be centralized and reused by login, rate
  limit, audit, and security events.

## 15. Implementation Steps

1. Keep already approved route-auth/collision hardening green and update its
   evidence.
2. Close P0 blockers one slice at a time, starting with API token indexed lookup.
3. Add a separate GUI slice plan before creating a frontend package.
4. Add a separate live E2E evidence slice plan before changing release scripts.
5. Clear `problem.md` and gap analysis only after Reviewer approves matching
   evidence.

## 16. Verification Plan

Run the smallest relevant command per slice, then the release gates:

```sh
go -C backend test ./internal/platform -run 'Route|Internal|ServiceAuth|Admin|Policy' -count=1
go -C backend test ./internal/services -run 'Catalog|Internal|Command|Contract' -count=1
go -C backend test ./... -count=1
go -C backend vet ./...
go -C backend build ./...
bash backend/scripts/ci-security-gate.sh quick
TEST_MINIO_PORT=29000 TEST_MINIO_CONSOLE_PORT=29001 bash backend/scripts/ci-security-gate.sh beta-rc
```

When the GUI exists, add:

```sh
npm test
npm run build
npm run e2e
```

Live evidence must also include Kubernetes rollout, smoke, rollback, redeploy,
backup/restore, WebRTC browser E2E, and failure-injection results.

## 17. Rollback Plan

Each slice must be independently revertible. Runtime rollback uses image/config
rollback plus reconciliation; database restore is reserved for disaster
recovery, not normal release rollback. GUI rollback removes the static frontend
artifact and any GUI-only route wiring.

## 18. Risks and Tradeoffs

- Full External GA is larger than one PR; forcing it into one diff would hide
  risk.
- A REST/OpenAPI GUI is less fashionable than adding WebRPC, but it avoids a new
  transport layer until there is a proven gap.
- Local agents may not have staging credentials; missing live evidence remains
  an open blocker rather than being waived.

## 19. Reviewer Checklist

- Requirement fit: External GA blockers, Web GUI, live E2E, and checklist
  closure are directly tracked.
- Scope control: every implementation PR is a small approved slice.
- SOLID: domain, transport, infrastructure, and GUI concerns stay separate.
- 12-Factor: dependencies declared, config externalized, logs as streams, build
  and run separated.
- CNCF/cloud-native: reuse Kubernetes, OIDC, OpenTelemetry, OCI registry, and
  secret-management integrations.
- Tests: focused tests plus release gates are recorded with evidence.
- Docs: `problem.md` and gap analysis are cleared only when evidence exists.

## 20. Status

Status: Changes Requested

Reviewer Agent requested that this remain a roadmap and that implementation move
through smaller approved slice plans.
