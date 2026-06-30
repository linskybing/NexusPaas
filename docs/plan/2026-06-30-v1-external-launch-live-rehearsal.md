# V1 External Launch Live Rehearsal

## 1. Objective

Run the existing production-beta live rehearsal harness to collect V1 external
launch evidence for external registry promotion, live Secret presence, migration
apply/validate, 8-unit deploy/smoke, and per-unit rollback/redeploy.

## 2. Background

`problem.md`, `gap.md`, and the GA trace matrix keep V1 external production
launch open because external staging evidence is still missing. The repository
already has `backend/scripts/production-beta-live-rehearsal.sh`; this slice must
reuse it instead of creating another harness.

## 3. Source References

- `backend/scripts/production-beta-live-rehearsal.sh`
- `backend/docs/beta-launch-readiness.md`
- `problem.md`
- `gap.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 4. Assumptions

- A non-local external Kubernetes context is available when the rehearsal runs.
- The candidate backend image is digest-pinned and hosted in an external registry.
- Required runtime Secrets already exist in the target namespace.
- Secret evidence is limited to names, keys, and provenance; values are never printed.

## 5. Non-Goals

- No new rehearsal script.
- No runtime code changes unless the harness exposes a concrete defect.
- No ledger status upgrade without a successful live run and Reviewer approval.
- No GPU-hardware or frontend/WebRTC closure.

## 6. Current Behavior

The harness performs guarded live staging mutation only when explicitly opted in
with `LIVE_STAGING_REHEARSAL=1`, rejects local contexts and local image
registries, renders the 8-unit production-beta topology, checks Secret presence,
runs migration jobs, deploys the candidate image, smokes each unit, and rehearses
per-unit rollback/redeploy.

## 7. Target Behavior

A successful run produces `production-beta-live-rehearsal-report.md` plus TSV
artifacts for previous images, Secret presence, migration jobs, rollouts, smoke
checks, and rollback/redeploy. Ledgers are updated only after Reviewer accepts
the evidence wording.

## 8. Affected Domains

Release evidence and documentation only.

## 9. Affected Files

- `docs/plan/2026-06-30-v1-external-launch-live-rehearsal.md`
- `problem.md`, `gap.md`, and `docs/acceptance/ga-acceptance-trace-matrix.md`
  only after successful live evidence and Reviewer approval.

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

No schema changes. The live harness may run existing `apply-migrations` and
`validate-migrations` admin jobs against staging.

## 12. Configuration Changes

No repository config changes. The operator must provide live environment
variables: `LIVE_STAGING_REHEARSAL=1`, `KUBE_CONTEXT`, digest-pinned
`CANDIDATE_IMAGE`, registry promotion evidence or copy inputs, and registry scan
evidence.

## 13. Observability Changes

None. The harness records evidence artifacts under `ARTIFACT_DIR`.

## 14. Security Considerations

Do not print Secret values, registry credentials, API keys, service keys, or scan
tokens. Secret verification is presence/provenance only.

## 15. Implementation Steps

1. Confirm current git state and preserve existing `problem.md` edits.
2. Confirm live rehearsal inputs are present and the kube context is non-local.
3. Run `backend/scripts/production-beta-live-rehearsal.sh`.
4. If the harness succeeds, submit generated artifacts to Reviewer Agent.
5. Update ledgers only with Reviewer-approved wording.
6. If the harness fails for environment readiness, record the blocker and stop.
7. If the harness exposes a code/deploy defect, create a separate narrow plan.

## 16. Verification Plan

```sh
git status --short --branch
kubectl config current-context
LIVE_STAGING_REHEARSAL=1 \
KUBE_CONTEXT=<external-context> \
CANDIDATE_IMAGE=<external-registry>/nexuspaas/backend@sha256:<digest> \
PROMOTION_EVIDENCE=<path-or-url> \
REGISTRY_SCAN_STATUS=Success \
bash backend/scripts/production-beta-live-rehearsal.sh
rg -n "V1 external production launch|8-unit|Secret readiness|migration/rollback|external registry" problem.md gap.md docs/acceptance/ga-acceptance-trace-matrix.md
git diff --check
```

## 17. Rollback Plan

If the live run starts and fails after mutation, use the harness' recorded
previous images and generated report artifacts to restore the prior deployment
state. Do not edit ledgers to claim success.

## 18. Risks and Tradeoffs

The harness intentionally refuses local contexts and local registries. That
blocks convenience runs but prevents false V1 external launch evidence.

## 19. Reviewer Checklist

- [ ] Existing harness is reused.
- [ ] Required live inputs are present and non-local.
- [ ] Secret evidence excludes values.
- [ ] Generated artifacts prove deploy, smoke, migrations, and rollback.
- [ ] Ledgers do not overclaim local/static evidence as live GA proof.

## 20. Status

Status: Approved
