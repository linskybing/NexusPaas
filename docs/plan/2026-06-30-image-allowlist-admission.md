# Plan + Record: In-code image allow-list submit admission

Status: Approved

_Agent-workflow record for the 2026-06-30 code change. Plan + Reviewer + Code
roles all run by Claude Code (Codex Code Agent not used) — fallback recorded per
[`workflow.md`](../agents/workflow.md)._

## Why

The High IMG gap lists "allow-list admission" as unmet. Scheduler submit admission
parsed PodSpec containers for GPU/CPU/secret/socket guards but never checked
container **images** against the project's published allow-list. This adds in-code
defense-in-depth, gated by the existing `K8S_IMAGE_CHECK_ENABLED` (no new config),
default-off so existing submits are unchanged.

## What

When `K8S_IMAGE_CHECK_ENABLED=true`, `reviewSubmitAdmission` → `evaluateSubmitAdmission`
rejects (`403`) any workload container/init image not matched by an enabled,
non-deleted allow rule for the project (`project_id` or `*`). Matching is
normalized `image_reference` (fallback `repository`+`tag`), `:latest`-defaulted.
The scheduler reads the allow-list (`image-registry-service:image_allow_lists`)
through the established owner-read contract seam; in `SERVICE_NAME=all` it resolves
locally, and the remote contract + fixture keep the isolation/dependency/fixture
guards green.

## Files

- `schedulerquota/read_contracts.go` — `imageAllowListsResource` const +
  `ListImageAllowRules` on `admissionReader` and both impls.
- `schedulerquota/admission.go` — `EnforceImageAllowList` on the request, set from
  `Config.ImageCheckEnabled`; call `enforceAdmissionImageAllowList` in the evaluator.
- `schedulerquota/admission_image.go` (new) — image extraction (reuses
  `admissionResourcePodSpecs` + `containersFromSpec`), allow-list matching, deny.
- `schedulerquota/admission_resources.go` — renamed `admissionRuntimeSocketPodSpecs`
  → `admissionResourcePodSpecs` (shared general PodSpec enumerator).
- `imageregistry/handler.go` — `RegisterReadContract(projectImagesResource,
  "/internal/image-registry/image-allow-lists", "")` (list-only owner route).
- `platform/service_client.go` — `domainReadContracts` entry.
- `services/catalog.go` — owner-read dependency (kept sorted).
- `contracts/fixtures/owner-read/v1/scheduler-image-allow-lists.json` (new) +
  `contracts/owner_read_fixtures_test.go` (`want`/`wantResources`/`wantSeen`).
- `platform/owner_read_fixtures_test.go` — fixture list, contract list, SERVICE_URL,
  and list-only-aware call-count assertion.
- `services/service_isolation_test.go` — sorted dep list + image-registry SERVICE_URL.
- `schedulerquota/admission_test.go` — `TestSubmitAdmissionEnforcesImageAllowList`
  (deny non-allow-listed, allow allow-listed, flag-off bypass).
- Docs: `problem.md` §2 IMG, `gap.md` IMG note, trace-matrix IMG Evidence Scope —
  IMG stays `Open`.

## Out of scope (deliberate)
- OCI reference equivalence beyond normalized ref / repository+tag (`ponytail:`
  comment marks the upgrade path).
- External OPA/Gatekeeper parity + live cluster enforcement — infra-gated, Open.
- No new dependency.

## Verification (all green this pass)
- `go build ./...` / `go vet ./...` / `gofmt -l` — clean.
- `go test ./...` — green; `TestSubmitAdmissionEnforcesImageAllowList` (3 subtests),
  owner-read fixture guards (contracts + platform), dependency-inventory +
  isolation guards all pass.
- `go test -race` (affected packages) — green.
- `python3 docs/tests/verify_ga_acceptance_trace_matrix.py` — green; IMG `Open`.

## Reviewer verdict

Opt-in + default-off ⇒ no behavior change unless enabled. Cross-service read goes
through the established owner-read seam with full contract/fixture/guard coverage.
No infra-gated P0 relabeled closed or live; IMG stays `Open` with external/live
enforcement explicitly noted as remaining. **Status: Approved.**
