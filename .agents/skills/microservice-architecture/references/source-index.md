# Source Index

Use this file to anchor microservice recommendations in current authoritative sources. Prefer these
sources over vendor blogs, framework tutorials, or generic opinion pieces.

## Authority Order

1. Standards and security authorities: NIST, OWASP.
2. Cloud architecture centers: Microsoft Azure Architecture Center, AWS Well-Architected and
   Prescriptive Guidance.
3. Widely cited architecture authors: Martin Fowler and Thoughtworks material.
4. Open standards projects: OpenTelemetry and CNCF.
5. Twelve-Factor App for runtime and deployability principles.

## Core Sources

- Microsoft Azure Architecture Center, "Microservices architecture style":
  https://learn.microsoft.com/en-us/azure/architecture/guide/architecture-styles/microservices
- Microsoft Azure Architecture Center, "Design a microservices architecture":
  https://learn.microsoft.com/en-us/azure/architecture/microservices/design/
- Microsoft Azure Architecture Center, "Design patterns for microservices":
  https://learn.microsoft.com/en-us/azure/architecture/microservices/design/patterns
- AWS Well-Architected, "REL03-BP01 Choose how to segment your workload":
  https://docs.aws.amazon.com/wellarchitected/latest/framework/rel_service_architecture_monolith_soa_microservice.html
- AWS Prescriptive Guidance, "Cloud design patterns, architectures, and implementations":
  https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/introduction.html
- Martin Fowler, "Microservices": https://martinfowler.com/articles/microservices.html
- Martin Fowler, "Microservice Prerequisites":
  https://martinfowler.com/bliki/MicroservicePrerequisites.html
- Martin Fowler, "Monolith First": https://martinfowler.com/bliki/MonolithFirst.html
- Martin Fowler, "Bounded Context": https://martinfowler.com/bliki/BoundedContext.html
- OWASP Microservices Security Cheat Sheet:
  https://cheatsheetseries.owasp.org/cheatsheets/Microservices_Security_Cheat_Sheet.html
- OWASP Zero Trust Architecture Cheat Sheet:
  https://cheatsheetseries.owasp.org/cheatsheets/Zero_Trust_Architecture_Cheat_Sheet.html
- NIST SP 800-204C: https://csrc.nist.gov/pubs/sp/800/204/c/final
- OpenTelemetry concepts and signals: https://opentelemetry.io/docs/concepts/
  https://opentelemetry.io/docs/concepts/signals/
- Twelve-Factor App config, backing services, processes, logs: https://12factor.net/config
  https://12factor.net/backing-services https://12factor.net/processes https://12factor.net/logs

## How To Use Sources

- Use Azure and AWS for pragmatic design patterns and trade-offs.
- Use Fowler to challenge premature microservice adoption and boundary quality.
- Use OWASP and NIST for security, DevSecOps, policy-as-code, and service mesh guidance.
- Use OpenTelemetry for vendor-neutral telemetry language.
- Use Twelve-Factor for deployability, config, state, and log handling.

## Source Hygiene

- Treat provider-specific implementation examples as examples, not mandates.
- Avoid copying source text into architecture output; paraphrase and cite.
- Re-check web sources when the user asks for "latest" guidance or when a recommendation depends on
  a product, version, service availability, or law.
