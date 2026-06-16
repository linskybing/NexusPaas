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

## Microservice Refactor Principle

Prefer gradual extraction over big-bang rewrite.

Service boundaries must be justified by:

* Business capability
* Data ownership
* Deployment independence
* Runtime responsibility
* Failure isolation
* Observability