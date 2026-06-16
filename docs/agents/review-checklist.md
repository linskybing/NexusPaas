# Review Checklist

Reviewer Agent is the quality gate.

Reviewer Agent reviews both plans and implementations.

## Plan Review Checklist

| Category | Question | Result |
|---|---|---|
| Requirement Fit | Does the plan directly satisfy the requirement? | Pass / Fail |
| Scope Control | Is the scope small enough? | Pass / Fail |
| Non-Goals | Are non-goals explicit? | Pass / Fail |
| Architecture | Is the design consistent with the backend refactor direction? | Pass / Fail |
| Microservice Boundary | Is the service boundary justified? | Pass / Fail |
| API Contract | Are API changes documented? | Pass / Fail |
| Data Ownership | Is database ownership clear? | Pass / Fail |
| Config | Are configs externalized? | Pass / Fail |
| Observability | Are logs, metrics, or traces considered? | Pass / Fail |
| Security | Are auth, authorization, and secrets considered? | Pass / Fail |
| Testing | Is verification concrete? | Pass / Fail |
| Rollback | Is rollback realistic? | Pass / Fail |
| Simplicity | Is the approach over-engineered? | Pass / Fail |
| Surgical Change | Are affected files limited? | Pass / Fail |

## Implementation Review Requirements

Reviewer Agent must produce these tables.

### Requirement Traceability

| Requirement | Evidence in Plan | Evidence in Diff | Status |
|---|---|---|---|
| `<requirement>` | `<plan section>` | `<file/function/test>` | Pass / Fail |

### SOLID Compliance

| Principle | Required Evidence | Actual Evidence | Risk / Gap | Status |
|---|---|---|---|---|
| Single Responsibility | Each module has one clear reason to change | `<evidence>` | `<risk>` | Pass / Fail |
| Open/Closed | New behavior avoids unnecessary modification of stable code | `<evidence>` | `<risk>` | Pass / Fail |
| Liskov Substitution | Interfaces are substitutable without hidden assumptions | `<evidence>` | `<risk>` | Pass / Fail |
| Interface Segregation | Interfaces are small and client-specific | `<evidence>` | `<risk>` | Pass / Fail |
| Dependency Inversion | High-level policy avoids direct dependency on low-level details | `<evidence>` | `<risk>` | Pass / Fail |

### 12-Factor App Compliance

| Factor | Required Evidence | Actual Evidence | Risk / Gap | Status |
|---|---|---|---|---|
| Codebase | Service ownership is clear | `<evidence>` | `<risk>` | Pass / Fail |
| Dependencies | Dependencies are explicitly declared | `<evidence>` | `<risk>` | Pass / Fail |
| Config | Config is externalized, not hardcoded | `<evidence>` | `<risk>` | Pass / Fail |
| Backing Services | External resources are attached services | `<evidence>` | `<risk>` | Pass / Fail |
| Build, Release, Run | Build, release, and runtime are separated | `<evidence>` | `<risk>` | Pass / Fail |
| Processes | Runtime is stateless where possible | `<evidence>` | `<risk>` | Pass / Fail |
| Port Binding | Service exposes itself through configured port | `<evidence>` | `<risk>` | Pass / Fail |
| Concurrency | Scale-out is process-based | `<evidence>` | `<risk>` | Pass / Fail |
| Disposability | Startup and shutdown are safe | `<evidence>` | `<risk>` | Pass / Fail |
| Dev/Prod Parity | Environments remain similar | `<evidence>` | `<risk>` | Pass / Fail |
| Logs | Logs are emitted as event streams | `<evidence>` | `<risk>` | Pass / Fail |
| Admin Processes | Admin tasks run as one-off processes | `<evidence>` | `<risk>` | Pass / Fail |

### Verification Results

| Command | Purpose | Result | Notes |
|---|---|---|---|
| `<command>` | `<why this was run>` | Pass / Fail / Not Run | `<notes>` |

### Required Fixes

| Priority | Issue | Required Fix | Blocking |
|---|---|---|---|
| High / Medium / Low | `<issue>` | `<fix>` | Yes / No |

## Approval Format

Reviewer Agent must end with one of:

```text
Status: Approved
```

or:

```text
Status: Changes Requested
```