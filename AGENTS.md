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
