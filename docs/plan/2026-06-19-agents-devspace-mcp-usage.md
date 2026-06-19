# DevSpace MCP Agent Instructions

Status: Approved

## Goal

Document how repository agents should use the installed DevSpace MCP endpoint for this workspace.

## Scope

- Update `AGENTS.md` only.
- Add concise agent-facing instructions for the installed direct DevSpace MCP setup.
- Include connection URL, expected authentication flow, allowed workspace root, operational prerequisites, security boundaries, and rollback pointers.

## Non-Goals

- Do not change application code.
- Do not change Cloudflare, DevSpace, or OpenAI resources.
- Do not commit secrets, owner tokens, tunnel tokens, or API keys.

## Implementation Steps

1. Add a new `DevSpace MCP` section to `AGENTS.md`.
2. State that agents should use the ChatGPT connector/app named `DevSpace MCP Direct` when available.
3. Document the MCP URL as `https://mcp.sky-lab.uk/mcp`.
4. Document that DevSpace exposes `/Users/sky/workspaces` and requires DevSpace Owner approval/password.
5. Document local runtime prerequisites:
   - `screen` session `devspace-mcp`
   - `screen` session `cloudflared-devspace-mcp`
   - DevSpace local origin `http://127.0.0.1:7676/mcp`
6. Document security rules:
   - Never print or commit `~/.devspace/auth.json`.
   - Never print or commit `~/.cloudflared/devspace-mcp.token`.
   - Treat the endpoint as owner-approved remote filesystem/tool access.
   - Do not broaden Cloudflare Access or DNS exposure without explicit user approval.
7. Document quick verification and stop commands.

## Verification

- Run `git diff --check`.
- Read the updated `AGENTS.md` section and confirm it is concise, actionable, and contains no secrets.

## Risks

- The endpoint is publicly reachable at `/mcp`; security relies on DevSpace OAuth/Owner approval.
- Over-documenting operational details in the entrypoint file could make `AGENTS.md` noisy, so the section should remain short.

## Reviewer Result

Status: Approved

The plan is limited to documentation, avoids secrets, preserves the existing Cloudflare/DevSpace setup, and has a clear verification path.

## Implementation Review

Status: Approved

- Requirement fit: `AGENTS.md` now tells agents how to use the installed DevSpace MCP connector.
- Approved-plan alignment: Only `AGENTS.md` was updated after approval.
- Security: No secrets, tokens, passwords, or API keys were added.
- Tests/build: `git diff --check` passed. Backend build, SonarScanner, and service tests were not required because this is documentation-only.
- Diff scope: Limited to `AGENTS.md` and this plan document.
