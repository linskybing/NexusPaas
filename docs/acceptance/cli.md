# Core Feature Area H: NexusPaaS CLI

Part of the [GA Acceptance docs](README.md).

## Goal

The CLI is the main power-user interface.

The CLI name is `nexus` (the configured brand binary; see [naming](naming.md)).

The CLI must hide API complexity and make image build, project selection,
deployment, logs, streaming, and usage easy.

## Required Commands

```text
nexus login
nexus logout
nexus whoami

nexus project list
nexus project use <project-id>

nexus image build \
  --project <project-id> \
  --name my-app \
  --tag v1 \
  --dockerfile Dockerfile \
  --context . \
  --cpu 4 \
  --memory 8Gi \
  --timeout 30m

nexus image list --project <project-id>
nexus image inspect <image-ref>

nexus deploy -f k8s.yaml --project <project-id> --queue gpu-high
nexus job list --project <project-id>
nexus job status <job-id>
nexus job logs <job-id>
nexus job cancel <job-id>

nexus stream open <job-id>

nexus usage user --from <time> --to <time>
nexus usage project <project-id> --from <time> --to <time>
nexus usage group <group-id> --from <time> --to <time>
```

## Login Modes

| Mode | Purpose |
|---|---|
| OIDC Device Code or PKCE | Human login |
| API token | Automation and CI |
| Short-lived session token | CLI session |
| Refresh token | Stored securely if policy allows |

Tokens should be stored in OS keychain where available.

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| CLI-001 | CLI binary is named `nexus` (configured brand binary). |
| CLI-002 | `nexus login` completes interactive login. |
| CLI-003 | `nexus whoami` shows current user and active Project. |
| CLI-004 | User can list only authorized Projects. |
| CLI-005 | `nexus project use` sets default Project. |
| CLI-006 | `nexus image build --context .` creates build context automatically. |
| CLI-007 | CLI honors `.dockerignore`. |
| CLI-008 | CLI refuses build without CPU/RAM/timeout. |
| CLI-009 | CLI can list Project images and show Harbor/deleted/scan-failed status. |
| CLI-010 | CLI can submit ConfigFile and show admission result. |
| CLI-011 | CLI shows clear rejection reason for image allow-list failure. |
| CLI-012 | CLI shows clear rejection reason for quota/Plan/Queue failure. |
| CLI-013 | CLI can tail Job logs. |
| CLI-014 | CLI can cancel Job idempotently. |
| CLI-015 | CLI can open stream session URL. |
| CLI-016 | CLI can query usage by user/project/group according to permissions. |
| CLI-017 | CLI never prints tokens, registry secrets, TURN secrets, or signed URLs in normal logs. |
| CLI-018 | API token revoke immediately blocks CLI automation. |
| CLI-019 | CLI version is compatible with API version or provides upgrade warning. |
| CLI-020 | CLI documentation uses the configured brand naming only. |
