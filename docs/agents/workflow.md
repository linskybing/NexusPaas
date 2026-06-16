# Agent Workflow

This repository uses a controlled three-agent development workflow.

## Roles

| Role | Responsibility |
|---|---|
| Plan Agent | Creates the implementation plan only |
| Code Agent | Implements only the approved plan |
| Reviewer Agent | Reviews both the plan and implementation |

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