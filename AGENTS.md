# AGENTS.md

All implementation work must follow the three-agent workflow.

1. **Plan Agent** writes a verifiable implementation plan under `docs/plan/`.
2. **Reviewer Agent** reviews the plan and requests revisions until approved.
3. **Code Agent** implements only the approved plan, then submits the result back to Reviewer Agent.

No code change is complete until Reviewer Agent verifies requirement fit, approved-plan alignment, SOLID, 12-Factor App compliance, tests/build results, SonarScanner Quality Gate status, risks, and diff scope.

This repository follows a **microservices-first** structure. Each service should own its code, API, data model, config, tests, and deployment files. See `docs/agents/project-structure.md`.

Keep this file as the entry point only. Detailed rules live in:

* `docs/agents/workflow.md`
* `docs/agents/planning.md`
* `docs/agents/review-checklist.md`
* `docs/agents/coding-guidelines.md`
* `docs/agents/project-structure.md`

## Git & Pull Request Workflow

* Every large feature or major goal gets its own **feature branch** and **pull request** — never commit big changes straight to `main`.
* Branch names must be **short and descriptive** (e.g. `identity-data-boundary`, not a long sentence).
* Every PR description must explain **what** was done (the features/changes), **why** it was done (the motivation/problem), and the **implementation idea** (the approach taken).
* When merging a PR, **squash all commits into a single commit** to keep history clean.

See `docs/agents/workflow.md` for the full branch & PR conventions.

Default rule: keep changes simple, surgical, testable, microservice-ready, SonarScanner-clean, and aligned with the backend microservice architecture documented in `backend/docs/`.

## DevSpace MCP

Agents may use the installed ChatGPT app/connector named **DevSpace MCP Direct** to work with this local workspace through MCP.

* MCP URL: `https://mcp.sky-lab.uk/mcp`
* Allowed workspace root: `/Users/sky/workspaces`
* Authentication: DevSpace OAuth with Owner approval/password.
* Local origin: `http://127.0.0.1:7676/mcp`
* Required local sessions:
  * `screen` session `devspace-mcp`
  * `screen` session `cloudflared-devspace-mcp`

Before relying on the connector, verify the runtime:

```bash
screen -ls
curl -i https://mcp.sky-lab.uk/mcp
```

Expected public `/mcp` behavior is a DevSpace OAuth `401`, not a Cloudflare Access redirect. Cloudflare Access may still protect other paths on `mcp.sky-lab.uk`.

Security rules:

* Never print, commit, or paste `~/.devspace/auth.json`.
* Never print, commit, or paste `~/.cloudflared/devspace-mcp.token`.
* Treat DevSpace access as owner-approved remote filesystem and tool access to `/Users/sky/workspaces`.
* Do not broaden Cloudflare DNS, Tunnel, Access, WAF, or bypass rules without explicit user approval.

Operational commands:

```bash
# Stop the public MCP route
screen -S cloudflared-devspace-mcp -X quit

# Stop the local DevSpace server
screen -S devspace-mcp -X quit
```

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tools** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them. `codegraph_node` returns one symbol's source + callers, or reads a whole file with line numbers. If the tools are listed but deferred, load them by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` and `codegraph node <symbol-or-file>` print the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
