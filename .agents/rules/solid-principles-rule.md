# SOLID Principles Rule

Use these rules when designing, reviewing, refactoring, or extending application code. SOLID is a
set of design principles for keeping software understandable, flexible, testable, and maintainable.

These rules apply to modules, classes, functions, services, adapters, interfaces, and implicit
contracts. Treat them as design guidance, not as a reason to add speculative abstractions.

## General Rule

Code should keep responsibilities focused, protect stable behavior from unnecessary modification,
honor declared and implied contracts, expose only the capabilities callers need, and depend on
abstractions at architectural boundaries.

Prefer the simplest design that preserves clear change boundaries. Do not introduce inheritance,
interfaces, factories, dependency containers, or plugin points unless they remove real coupling or
make expected extension safer.

## Principles

1. **Single Responsibility Principle (SRP)** Keep together things that change for the same reason.
   Separate things that change for different reasons.

   - A module should have one clear purpose at its current level of abstraction.
   - Split behavior when one change category can break or force redeployment of unrelated behavior.
   - Do not mix policy, persistence, presentation, transport, formatting, and orchestration unless
     the combination is deliberately small and local.

2. **Open/Closed Principle (OCP)** Design stable code so it can be extended without repeatedly
   modifying the stable core.

   - Add new variants by composing, registering, or implementing clear extension points.
   - Avoid scattered conditionals that require editing many existing paths for every new case.
   - Do not over-engineer for unknown futures; apply this most strongly where variation already
     exists or is explicitly expected.

3. **Liskov Substitution Principle (LSP)** Implementations must preserve the contract expected by
   users of the abstraction.

   - A subtype, implementation, adapter, or duck-typed value must be usable wherever its abstraction
     is accepted.
   - Do not narrow valid inputs, weaken required outputs, change error semantics, or add surprising
     side effects behind the same interface.
   - If callers need type checks or special branches to stay correct, the abstraction is probably
     unclear or too broad.

4. **Interface Segregation Principle (ISP)** Keep interfaces small and shaped around caller needs.

   - Callers should not depend on methods, fields, events, permissions, or lifecycle hooks they do
     not use.
   - Prefer focused contracts over broad catch-all interfaces.
   - Split interfaces when unrelated clients change at different rates or require unrelated
     capabilities.

5. **Dependency Inversion Principle (DIP)** High-level policy should depend on abstractions, not
   low-level details.

   - Domain and orchestration code should depend on contracts at meaningful boundaries.
   - Details such as storage, transport, user interface, external services, clocks, randomness, and
     file systems should be replaceable through adapters when they cross a boundary.
   - Keep dependency direction pointing toward stable policy and away from volatile implementation
     details.

## Review Checklist

- Responsibilities are named clearly, and "and" in a module description is justified or split.
- New variants do not require modifying multiple unrelated existing branches.
- Implementations obey the same input, output, error, ordering, and side-effect expectations as the
  abstractions they implement.
- Interfaces expose only what their callers need.
- High-level behavior can be tested without depending on volatile low-level details.
- Abstractions are introduced only where they reduce real coupling or support known variation.
- The design remains simpler after applying SOLID than before applying it.

## Sources

- [Clean Coder: Solid Relevance](https://blog.cleancoder.com/uncle-bob/2020/10/18/Solid-Relevance.html)
- [Wikipedia: SOLID](https://en.wikipedia.org/wiki/SOLID)
- [DigitalOcean: SOLID Design Principles](https://www.digitalocean.com/community/conceptual-articles/s-o-l-i-d-the-first-five-principles-of-object-oriented-design)
