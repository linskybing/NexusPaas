# Storage API Fixtures Sonar Cleanup

Date: 2026-06-28
Status: Approved

## 1. Objective

Make the local SonarScanner Quality Gate pass by fixing the three new Sonar
test-maintainability issues reported in
`backend/internal/services/storage/api_fixtures_test.go`.

## 2. Background

The FastTransfer progress-to-storage kind E2E slice passed Go tests and build,
but SonarScanner failed the Quality Gate on `new_violations=3`. The violations
are in an existing storage API fixture test file, not in runtime code or the new
E2E file.

## 3. Source References

- `backend/internal/services/storage/api_fixtures_test.go`
- Sonar Quality Gate issue API for project `nexuspaas-backend`
- `backend/scripts/ci-security-gate.sh sonar`

## 4. Assumptions

- The Sonar issues are legitimate maintainability findings for test helpers.
- Fixing them by reusing existing helper logic is lower risk than suppressing
  rules.
- No fixture JSON or service contract behavior should change.

## 5. Non-Goals

- No runtime service changes.
- No API/contract fixture content changes.
- No Sonar rule suppression or quality profile edits.
- No broad cleanup of unrelated storage fixture tests.

## 6. Current Behavior

Two helper pairs have identical implementations, and one cache binding route
metadata helper has cognitive complexity above the configured Sonar threshold.

## 7. Target Behavior

The duplicate helpers should delegate to one shared helper, and the complex
route metadata helper should delegate narrow checks to smaller helpers while
preserving the same assertions.

## 8. Affected Domains

- `storage-service` tests only.
- Local Sonar Quality Gate verification.

## 9. Affected Files

Update:

- `backend/internal/services/storage/api_fixtures_test.go`

No ledger update is required unless verification materially changes AC/gap
evidence.

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

None.

## 14. Security Considerations

None. The change stays in test helper code and does not alter auth assertions.

## 15. Implementation Steps

1. Add one shared external API fixture metadata helper and make cache binding
   and FastTransfer helpers call it.
2. Add one shared status/event helper and make cache binding and FastTransfer
   helpers call it.
3. Split cache binding route metadata checks into smaller helper functions for
   resource/action/path, ID param, route flags, and fixture parity.
4. Run gofmt and focused tests.
5. Rerun SonarScanner Quality Gate.

## 16. Verification Plan

```bash
cd backend
go test ./internal/services/storage -run ExternalAPI -count=1
go test ./internal/services/storage/... -count=1
go test ./... -count=1
go build ./...
cd ..
git diff --check
bash backend/scripts/ci-security-gate.sh sonar
```

## 17. Rollback Plan

Revert the helper refactor in
`backend/internal/services/storage/api_fixtures_test.go`.

## 18. Risks and Tradeoffs

The main risk is accidentally weakening fixture assertions while deduplicating
helpers. The mitigation is to keep every existing check, only move checks into
shared helpers, and run the storage fixture tests.

## 19. Reviewer Checklist

- [ ] The change touches only storage API fixture test helpers.
- [ ] No fixture JSON or production code changes.
- [ ] Duplicate helper implementations are removed by delegation, not
      suppression.
- [ ] Cache binding route assertions still check resource, action, method,
      path, auth flags, ID param, state-changing flag, service auth,
      policy bypass, external adapter, and fixture route parity.
- [ ] Sonar Quality Gate passes.

## 20. Status

Status: Approved
