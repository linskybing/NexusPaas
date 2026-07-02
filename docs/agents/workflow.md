# Agent Workflow

This repository uses a controlled three-agent development workflow.

## Roles

| Role | Responsibility |
|---|---|
| Plan Agent | Creates the implementation plan only |
| Code Agent | Implements only the approved plan |
| Reviewer Agent | Reviews both the plan and implementation |

## Tool Assignment

When quota and local tooling are sufficient:

| Role | Default Tool |
|---|---|
| Plan Agent | Claude Code |
| Reviewer Agent | Claude Code |
| Code Agent | Codex |

If Claude Code or Codex is unavailable, agents must keep the same role
separation, record the fallback in the plan or review, and continue only when
the normal approval gates are satisfied.

## Required Flow

```text
User Requirement
  -> Plan Agent writes docs/plan/<task>.md
  -> Reviewer Agent reviews the plan
  -> Plan Agent revises until approved
  -> Code Agent implements the approved plan
  -> Reviewer Agent reviews the implementation
  -> Code Agent fixes issues if needed
  -> Reviewer Agent approves completion
````

## Approval Rules

Implementation must not begin until the plan is approved.

A task is not complete until Reviewer Agent marks the implementation as approved.

For GA acceptance-criteria clearance, work in this order:

1. V1 external production launch blockers.
2. Non-deferred Full GA gaps.
3. Deferred GPU-hardware or frontend/WebRTC items only after the required
   environment exists.

`blocker-ledger.md`, `gap-tracker.md`, and `docs/acceptance/ga-acceptance-trace-matrix.md`
must be updated only after matching evidence exists. Local, static, fixture, or
single-cluster evidence must not be described as live external GA proof.

**Owner-decision exception (ADR 0008, 2026-07-02):** the project owner may
close a launch row on non-external evidence only through an explicit, recorded
launch decision. Such rows must cite ADR 0008, keep their true evidence scope
visible (e.g. "kind-tier, owner-accepted staging"), and must never be worded as
external proof. The V1 launch sign-off uses this exception: staging =
owner-accepted kind cluster; registry evidence (ghcr.io promote/rollback) is
genuinely external.

## Status Values

Use only these status values:

```text
Status: Draft
Status: Changes Requested
Status: Approved
```

## Source Context

Agents should use the following context when relevant:

* Current backend source code
* Architecture documentation under `backend/docs/`
* Existing tests and deployment files

## Branch & PR Conventions

* **One goal per branch.** Each large feature or major goal is developed on its own feature branch off `main`.
* **Branch naming:** short, lowercase, hyphenated, descriptive of the goal (e.g. `identity-data-boundary`). Avoid long or sentence-like names.
* **Pull requests:** open a PR for every feature branch; do not push large changes directly to `main`.
* **PR description (required):** every PR must clearly cover three things:
  * **What** — the features or changes delivered.
  * **Why** — the motivation, problem, or goal that prompted the change.
  * **How** — the implementation idea / approach taken, including key trade-offs or rollback notes.
* **Squash merge:** merge each PR by squashing its commits into a single commit so `main` keeps a clean, linear history.

## Microservice Refactor Principle

Prefer gradual extraction over big-bang rewrite.

Service boundaries must be justified by:

* Business capability
* Data ownership
* Deployment independence
* Runtime responsibility
* Failure isolation
* Observability
