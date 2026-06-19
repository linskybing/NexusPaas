# DevSpace MCP Cloudflare Tunnel

## 1. Objective

Expose the local DevSpace MCP server for `/Users/sky/workspaces` at `https://mcp.sky-lab.uk/mcp` through a dedicated Cloudflare Tunnel, protected by Cloudflare Access and DevSpace Owner OAuth/password.

## 2. Background

DevSpace provides local coding-agent capabilities over MCP. It is intended to run on the developer machine and be exposed through a tunnel or reverse proxy. The Cloudflare account already has an active `sky-lab.uk` zone and existing tunnels for other hostnames; this setup must not modify those existing tunnel routes.

## 3. Source References

- User-approved plan: "DevSpace MCP over Cloudflare Tunnel".
- DevSpace docs: `https://github.com/Waishnav/devspace`, `docs/setup.md`, `docs/configuration.md`, and `docs/security.md`.
- Cloudflare One docs for Tunnel DNS records, remote tunnel configuration, and Access self-hosted applications.
- Local Cloudflare skill references for Tunnel configuration, token-based tunnels, and Tunnel troubleshooting.

## 4. Assumptions

- The target zone is `sky-lab.uk`, and `mcp.sky-lab.uk` is the chosen hostname.
- The Cloudflare Access owner identity is configured outside the repository.
- DevSpace runs locally on `127.0.0.1:7676`.
- ChatGPT is the primary remote MCP client.
- `cloudflared` may be installed locally if missing.

## 5. Non-Goals

- Do not alter existing `tickets.sky-lab.uk`, `grafana.sky-lab.uk`, staging, or WARP tunnel configuration.
- Do not expose the MCP endpoint without Cloudflare Access unless a separate approval explicitly accepts that downgrade.
- Do not add backend code, database changes, Kubernetes manifests, launchd automation, or CI workflow changes.
- Do not commit DevSpace auth secrets, Cloudflare tunnel tokens, or local runtime state.

## 6. Current Behavior

`@waishnav/devspace` is installed globally, but no `~/.devspace/config.json` or `~/.devspace/auth.json` exists. `cloudflared` is not available in `PATH`. The Cloudflare zone `sky-lab.uk` is active, and no DNS record exists for `mcp.sky-lab.uk`.

## 7. Target Behavior

The local machine runs DevSpace on `http://127.0.0.1:7676`. A dedicated Cloudflare Tunnel named `devspace-mcp` routes `mcp.sky-lab.uk` to that local service. Cloudflare DNS contains a proxied CNAME for `mcp.sky-lab.uk`, and Cloudflare Access requires the owner identity before traffic reaches DevSpace. MCP clients use `https://mcp.sky-lab.uk/mcp`.

## 8. Affected Domains

- Local developer tooling.
- Cloudflare Tunnel, DNS, and Access configuration.
- Repository documentation for the required plan/review workflow.

## 9. Affected Files

- `docs/plan/2026-06-19-devspace-mcp-cloudflare-tunnel.md`

No production code files are affected.

## 10. API / Contract Changes

No NexusPaas runtime API changes. The externally visible MCP endpoint is `https://mcp.sky-lab.uk/mcp`, backed by DevSpace's MCP/OAuth discovery endpoints.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

- Local DevSpace config: allowed root `/Users/sky/workspaces`, local port `7676`, public base URL `https://mcp.sky-lab.uk`.
- Cloudflare Tunnel: `devspace-mcp` with Cloudflare-managed config source.
- Cloudflare ingress: `mcp.sky-lab.uk -> http://127.0.0.1:7676`, followed by `http_status:404`.
- Cloudflare DNS: proxied CNAME `mcp.sky-lab.uk -> <tunnel-id>.cfargotunnel.com`.
- Cloudflare Access: self-hosted app for `mcp.sky-lab.uk` with an allow policy for the approved owner identity.

## 13. Observability Changes

No application observability changes. Verification relies on `devspace doctor`, local process logs, Cloudflare tunnel status, Cloudflare DNS state, and Access behavior.

## 14. Security Considerations

DevSpace grants powerful local file and shell capabilities inside allowed roots. The configuration keeps the filesystem allowlist narrow, uses Cloudflare Access as the first gate, and keeps DevSpace Owner OAuth/password as the second gate. Tokens and owner credentials must remain outside git. If ChatGPT cannot pass Cloudflare Access, the endpoint must remain protected until a separate security downgrade is approved.

## 15. Implementation Steps

1. Add this plan and mark it approved after Reviewer Agent review.
2. Install `cloudflared` with Homebrew if it remains missing.
3. Initialize DevSpace local config for `/Users/sky/workspaces`, port `7676`, and `https://mcp.sky-lab.uk`.
4. Create a dedicated Cloudflare Tunnel named `devspace-mcp` with `config_src: cloudflare`.
5. Configure tunnel ingress for `mcp.sky-lab.uk` and the `http_status:404` catch-all.
6. Create a proxied DNS CNAME for `mcp.sky-lab.uk` pointing at `<tunnel-id>.cfargotunnel.com`.
7. Create a Cloudflare Access self-hosted app and allow policy for the owner identity.
8. Start `devspace serve` and `cloudflared tunnel --no-autoupdate run --token <TOKEN>` manually in background sessions for first validation.
9. Verify local, Cloudflare, Access, and endpoint behavior.

## 16. Verification Plan

```sh
node -v
npm view @waishnav/devspace version engines --json
devspace doctor
curl -i http://127.0.0.1:7676/mcp
git diff --check
```

Cloudflare verification:

- Confirm `mcp.sky-lab.uk` DNS record exists and is proxied.
- Confirm `devspace-mcp` tunnel config includes only the MCP hostname route and the 404 catch-all.
- Confirm the tunnel reaches `healthy` after `cloudflared` starts.
- Confirm unauthenticated `https://mcp.sky-lab.uk` traffic is blocked by Cloudflare Access before reaching DevSpace.

Manual client verification:

- Configure ChatGPT remote MCP with `https://mcp.sky-lab.uk/mcp`.
- Complete Cloudflare Access if the client supports it.
- Complete DevSpace Owner approval.
- Open `/Users/sky/workspaces`.

## 17. Rollback Plan

Stop the local `devspace` and `cloudflared` processes. Delete or disable the Cloudflare Access application, delete the `mcp.sky-lab.uk` DNS record, delete the `devspace-mcp` tunnel, and rotate/delete the DevSpace Owner token if exposed during testing.

## 18. Risks and Tradeoffs

- Some MCP clients may not support Cloudflare Access browser login or Access service-token headers.
- The local machine must keep both DevSpace and `cloudflared` running for the endpoint to work.
- A dedicated tunnel avoids blast radius to existing hostnames but adds one more Cloudflare object to operate.
- DevSpace shell access is intentionally powerful; the allowed root must remain narrow.

## 19. Reviewer Checklist

- Requirement fit: exposes DevSpace MCP at `https://mcp.sky-lab.uk/mcp`.
- Scope: no backend code, database, Kubernetes, or existing tunnel modifications.
- Architecture: uses Cloudflare Tunnel because the origin is a local service.
- Config: runtime config stays external to production code.
- Security: Cloudflare Access and DevSpace Owner OAuth/password are both required.
- Testing: local and Cloudflare verification steps are concrete.
- Rollback: external resources and local processes can be removed cleanly.

## 20. Status

Status: Approved
