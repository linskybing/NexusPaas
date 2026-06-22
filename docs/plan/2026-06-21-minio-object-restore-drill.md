# MinIO Object Storage Restore Drill

Status: Approved

## 1. Objective

Capture live OPS-008 evidence that NexusPaaS object storage can back up and restore an object by
using the existing MinIO deployment and its bundled `mc` client against a synthetic non-sensitive
object in the existing `media` bucket.

## 2. Background

`docs/acceptance/operations.md` requires OPS-008: object storage backup and restore drill passes.
The live `nexuspaas` namespace has a ready `minio` deployment, a `minio-data` PVC, and an existing
`media` bucket. The MinIO container already includes `mc`, `sha256sum`, `stat`, `curl`, and
`minio`, so this drill can use standard MinIO client commands rather than custom S3 code.

Official documentation references:

- MinIO `mc alias set`: https://docs.min.io/aistor/reference/cli/mc-alias/mc-alias-set/
- MinIO `mc cp`: https://docs.min.io/aistor/reference/cli/mc-cp/
- MinIO `mc rm`: https://docs.min.io/aistor/reference/cli/mc-rm/
- MinIO `mc ls`: https://docs.min.io/aistor/reference/cli/mc-ls/

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/ga-checklist.md`
- `gap.md`
- `problem.md`
- Live `nexuspaas` namespace `minio` deployment, service, PVC, and bucket state from `kubectl`

## 4. Assumptions

- The current `kubectl` context is the same live local/RKE2-style context used by prior GA
  evidence slices.
- The `minio` deployment is ready before the drill starts.
- The existing `media` bucket is the correct object-storage bucket for the current local stack.
- MinIO root credentials are read from pod environment variables but never printed.
- The synthetic object contains no secret, user, customer, or production data.

## 5. Non-Goals

- Do not modify or delete existing user/media objects.
- Do not claim OPS-007 Harbor restore, OPS-009 secret recovery, off-cluster object backup,
  versioned restore, bucket metadata restore, IAM/policy restore, encryption-at-rest validation,
  or full DR coverage.
- Do not add a new object-storage client dependency.
- Do not change application code, manifests, runtime config, secrets, or bucket policy.

## 6. Current Behavior

MinIO is ready in the live namespace and lists the `media` bucket. The GA ledgers currently mark
backup/restore as partial because OPS-006 has PostgreSQL evidence but OPS-008 object storage
restore is not yet evidenced.

## 7. Target Behavior

The live object storage has a successful restore-drill record:

1. Preflight captures context, namespace, MinIO image/readiness, `mc` availability, bucket list,
   `/data` free space, and local `/tmp` free space without printing credentials.
2. A temporary `MC_CONFIG_DIR` is used for the alias.
3. A non-sensitive synthetic payload is created under `/tmp`.
4. The payload is uploaded to `media/ops008/<timestamp>/payload.txt`.
5. The object is backed up to `/tmp`.
6. The remote object is deleted and confirmed absent.
7. The object is restored from the local backup to the same object path.
8. The restored object is downloaded and SHA-256 verified against the original payload.
9. The remote object, local payload, local backup, local restored copy, and `MC_CONFIG_DIR` are
   deleted.
10. Ledgers mark OPS-008 object restore drill as evidenced while preserving remaining DR gaps.

## 8. Affected Domains

- OPS-008 object storage backup and restore drill
- GA problem and gap tracking
- Live MinIO object storage operations

## 9. Affected Files

- `docs/plan/2026-06-21-minio-object-restore-drill.md`
- `gap.md`
- `problem.md`

## 10. API / Contract Changes

None.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

None.

## 13. Observability Changes

No permanent observability changes. Evidence will be collected from:

- `kubectl config current-context`
- `kubectl -n nexuspaas get deploy minio`
- `kubectl -n nexuspaas exec deploy/minio -- ...`
- `mc ls`, `mc cp`, `mc rm`, and SHA-256 comparisons
- cleanup readbacks for the remote object and local temp files

The full evidence table will be appended to this plan under `## 21. Implementation Evidence`.

## 14. Security Considerations

- Do not print MinIO access key or secret key.
- Use `MC_CONFIG_DIR=/tmp/mc-ops008-<timestamp>` and delete it after the drill.
- Use a synthetic object only.
- Scope `mc rm` to the exact temporary object path; do not use recursive bucket deletion.
- Delete all temporary local files after validation.
- Do not commit backup artifacts.

## 15. Implementation Steps

1. Capture preflight:
   - `kubectl config current-context`
   - namespace: `nexuspaas`
   - `minio` deployment readiness and image
   - `mc`, `sha256sum`, `stat`, and `curl` availability inside `deploy/minio`
   - `/data` free bytes from `df -Pk /data`
   - local `/tmp` free bytes inside the MinIO container from `df -Pk /tmp`
   - `mc ls local` shows the `media` bucket after alias setup
2. Abort if `/data` or `/tmp` free bytes are below 128 MiB.
3. Create a temporary `MC_CONFIG_DIR`, alias `local`, payload file, backup path, restored path, and
   object path `local/media/ops008/<timestamp>/payload.txt`.
4. Assert the object path does not already exist.
5. Upload the payload with `mc cp`.
6. Backup the object from MinIO to `/tmp` with `mc cp`.
7. Verify original payload SHA-256 equals backup SHA-256.
8. Delete the remote object with `mc rm <exact-object-path>`.
9. Confirm `mc stat <exact-object-path>` fails after delete.
10. Restore the object from local backup to the same MinIO object path with `mc cp`.
11. Download the restored object to `/tmp` with `mc cp`.
12. Verify restored SHA-256 equals original payload SHA-256.
13. Delete the remote object, local payload, local backup, restored copy, and `MC_CONFIG_DIR`.
14. Confirm the remote object and local temp artifacts are absent.
15. Update `gap.md` and `problem.md`: OPS-008 object restore drill is evidenced; OPS-007,
    OPS-009, off-cluster retention, versioned restore, bucket metadata/IAM restore, encrypted
    backup storage, and full DR remain open.
16. Submit the evidence and ledger updates to Reviewer Agent.

## 16. Verification Plan

- Preflight shows selected context, namespace, ready `minio` deployment, and required tools.
- `mc ls local` lists the `media` bucket.
- Free-space guards pass before object write/backup.
- The temporary object path is absent before upload.
- Upload, backup copy, delete, restore, and download commands exit successfully.
- Original, backup, and restored SHA-256 values match.
- `mc stat` fails after delete and after final cleanup.
- Local payload, backup, restored copy, and `MC_CONFIG_DIR` are absent after cleanup.
- `git diff --check` passes after ledger edits.
- Not applicable for this docs/live-evidence-only slice because no application code, tests, build
  scripts, manifests, or runtime config are changed: `go -C backend test ./...`, `npm --prefix
  frontend run test`, `npm --prefix frontend run build`, and SonarScanner. If the scope expands
  beyond docs/evidence, these gates must run.

## 17. Rollback Plan

If any step fails:

1. Stop the drill.
2. Attempt to remove only the exact temporary object path.
3. Delete the local payload, backup, restored copy, and `MC_CONFIG_DIR`.
4. Capture the failing stage and non-secret command output.
5. Do not run recursive delete or bucket-level cleanup.

## 18. Risks and Tradeoffs

- The drill writes one synthetic temporary object to the live `media` bucket and deletes it after
  validation.
- This proves object backup/delete/restore mechanics through MinIO S3 APIs, not remote retention,
  cross-cluster restore, versioned object recovery, IAM/policy recovery, or bucket metadata restore.
- Using the existing bucket avoids bucket policy changes, but it means the drill does not test
  bucket creation or full bucket restore.

## 19. Reviewer Checklist

- The drill only uses a synthetic temporary object and exact object paths.
- No credentials or object contents are printed.
- `mc rm` is not recursive and never targets the bucket root.
- The plan uses the existing MinIO `mc` client rather than custom object-storage code.
- Evidence is specific enough to verify OPS-008 without overstating full DR.
- Ledger updates preserve OPS-007, OPS-009, and broader DR gaps.

## 20. Status

Status: Approved. Reviewer Agent approved the plan before live object-storage commands ran.

## 21. Implementation Evidence

Executed on 2026-06-21 against `kubectl config current-context=default`, namespace
`nexuspaas`.

An initial local script attempt aborted during preflight because the MinIO image does not include
`awk`; no object was created. Follow-up readback showed no `ops008` object and no matching
`/tmp` artifacts. The successful run used host-side parsing for `df` output and only required
tools present in the MinIO container.

| Check | Evidence | Result |
|---|---|---|
| MinIO deployment | image `minio/minio:RELEASE.2025-04-08T15-41-24Z`; ready `1/1` | Pass |
| Tool availability | `mc sha256sum stat curl df ls rm mkdir` present in `deploy/minio` | Pass |
| Bucket listing | `media/` present | Pass |
| `/data` free space before drill | `611286564864` bytes | Pass |
| `/tmp` free space before drill | `611286564864` bytes | Pass |
| Temporary object path | `local/media/ops008/20260621151838/payload.txt` | Pass |
| Payload size | `46` bytes | Pass |
| Payload SHA-256 | `dc4c12603462b5385f6cbb676cff88ba6503e1aba4b42ef10224c0b276c76d5b` | Pass |
| Backup size | `46` bytes | Pass |
| Backup SHA-256 | `dc4c12603462b5385f6cbb676cff88ba6503e1aba4b42ef10224c0b276c76d5b` | Pass |
| Delete before restore | object absence check after exact-path `mc rm` passed | Pass |
| Restored size | `46` bytes | Pass |
| Restored SHA-256 | `dc4c12603462b5385f6cbb676cff88ba6503e1aba4b42ef10224c0b276c76d5b` | Pass |
| Restore validation | restored SHA matched original payload SHA | Pass |
| Remote object cleanup | independent readback returned `object_absent` | Pass |
| Local artifact cleanup | no `/tmp/mc-ops008-*` or `/tmp/nexuspaas-ops008-*` artifacts remained | Pass |
| MinIO readiness after drill | `1/1` | Pass |

This closes live OPS-008 synthetic object backup/delete/restore evidence only. OPS-007 Harbor
restore, OPS-009 secret recovery, off-cluster object retention, versioned object restore,
bucket metadata/IAM restore, encrypted backup storage, and full DR remain open.
