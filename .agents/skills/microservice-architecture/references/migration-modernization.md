# Migration And Modernization

Use this reference when moving from a monolith or SOA system toward microservices.

## Default Strategy

- Prefer a modular monolith when the domain is unclear or the team lacks distributed-system
  operations.
- Prefer incremental extraction over a big-bang rewrite.
- Start with high-value, low-coupling capabilities at the edge of the monolith.
- Keep the monolith stable while new capability moves out.
- Set measurable exit criteria for each migration slice.

## Strangler Fig Flow

1. Add a routing layer that can send selected traffic to either the monolith or the new service.
2. Extract one capability with clear ownership and limited dependencies.
3. Add an anti-corruption layer when old and new models differ.
4. Move reads, writes, and source-of-truth ownership deliberately.
5. Run reconciliation while old and new systems coexist.
6. Move consumers to the new contract.
7. Retire adapters, sync jobs, routes, and old data paths.

## Anti-Corruption Layer

- Use an ACL to translate between legacy models and new service contracts.
- Keep ACL behavior narrow and temporary.
- Avoid leaking legacy concepts into the new service domain model.
- Put an owner and removal condition on every ACL.
- Instrument ACL calls so hidden migration coupling is visible.

## Data Transition

- Identify current source of truth and target source of truth.
- Use events, outbox, CDC, or sync agents only with reconciliation and retirement.
- Avoid indefinite dual writes.
- Define conflict handling before both systems can update related data.
- Prove migration with counts, checksums, domain invariants, and sampled audits.

## Candidate Selection

Good first extractions:

- Low write contention.
- Clear API boundary.
- Independent scaling or release value.
- Limited direct database coupling.
- Strong business owner.

Poor first extractions:

- Central identity, billing, or authorization logic without mature controls.
- Workflows requiring many compensating transactions.
- Highly shared tables with unclear ownership.
- Code that is being rewritten for unclear product reasons.

## Migration Stop Conditions

- New service depends on direct reads from many monolith tables.
- User-visible consistency model is undefined.
- Rollback requires manual database surgery.
- No telemetry distinguishes monolith path from new service path.
- The team cannot name what will be deleted when migration completes.
