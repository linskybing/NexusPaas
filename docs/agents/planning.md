# Planning Rules

Plan Agent creates implementation plans only.

Plan Agent must not modify production code.

## Output Location

Every plan must be created under:

```text
docs/plan/
````

Recommended filename:

```text
docs/plan/YYYY-MM-DD-<task-name>.md
```

Example:

```text
docs/plan/2026-06-12-extract-job-service.md
```

## Required Plan Sections

Every plan must include:

```markdown
# <Task Name>

## 1. Objective

## 2. Background

## 3. Source References

## 4. Assumptions

## 5. Non-Goals

## 6. Current Behavior

## 7. Target Behavior

## 8. Affected Domains

## 9. Affected Files

## 10. API / Contract Changes

## 11. Database / Migration Changes

## 12. Configuration Changes

## 13. Observability Changes

## 14. Security Considerations

## 15. Implementation Steps

## 16. Verification Plan

## 17. Rollback Plan

## 18. Risks and Tradeoffs

## 19. Reviewer Checklist

## 20. Status
```

## Planning Requirements

Plan Agent must:

* Restate the requirement clearly.
* State assumptions explicitly.
* List non-goals.
* Keep scope small.
* Identify affected files.
* Identify affected APIs, database tables, configs, and deployment files.
* Define verifiable implementation steps.
* Include rollback strategy.
* Include test/build/lint verification commands.
* Justify microservice boundaries when extracting services.

## Microservice Boundary Checklist

When proposing a new service, answer:

| Question                                    | Required |
| ------------------------------------------- | -------- |
| What business capability owns this service? | Yes      |
| What data does it own?                      | Yes      |
| Which APIs does it expose?                  | Yes      |
| Which services call it?                     | Yes      |
| Can it be deployed independently?           | Yes      |
| Can it fail independently?                  | Yes      |
| How is it observed?                         | Yes      |
| How is it tested?                           | Yes      |
| How is it rolled back?                      | Yes      |

## Forbidden Planning Patterns

Do not propose:

* Big-bang rewrites
* Shared mutable database ownership
* Distributed transactions without strong justification
* New infrastructure without clear need
* Generic abstractions for one use case
* Refactors unrelated to the requirement