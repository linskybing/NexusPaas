# Naming and Branding Policy

Part of the [GA Acceptance docs](README.md).

## Naming Model

**NexusPaaS is the primary, default name** used in code, docs, CLI, labels,
domains, and metrics. A downstream deployment may be rebranded (for example to
**CSCC AI Platform**) **with no code change** by changing the brand
configuration / running the documented substitution. NexusPaaS remains the
upstream open-source repository and engineering project name.

Primary audience: platform engineers, operators, security reviewers, and release
owners.

## Rebrand Seam (single source of truth)

Every brand-bearing string flows through one substitution map. To produce a
rebranded launch build, change the brand config to the target column; nothing
else in the codebase needs to change.

| Token | NexusPaaS default (used everywhere) | CSCC swap value (example launch brand) |
|---|---|---|
| Product title | `NexusPaaS` | `CSCC AI Platform` |
| CLI binary | `nexus` | `cscc` |
| CLI examples | `nexus login`, `nexus image build`, `nexus deploy` | `cscc login`, `cscc image build`, `cscc deploy` |
| Browser title | `NexusPaaS` | `CSCC AI Platform` |
| User documentation | `NexusPaaS` | `CSCC AI Platform` |
| API client package | NexusPaaS client / NexusPaaS SDK | CSCC client / CSCC SDK |
| Label prefix | `nexuspaas.io/` | `cscc.ai/` |
| Registry domain | `registry.nexuspaas.io/<project>/<image>` | `registry.cscc.ai/<project>/<image>` |
| API domain | `api.nexuspaas.io` | `api.cscc.ai` |
| TURN domain | `turn.nexuspaas.io` | `turn.cscc.ai` |
| Namespace | `nexuspaas-system` | `cscc-system` or `cscc-ai-platform` |
| Metric prefix | `nexuspaas_` | `cscc_` |

### How to produce a rebranded launch build

1. Set the brand config to the target column above (product title, CLI binary,
   label prefix, domains, namespace, metric prefix) from one source — do not
   scatter brand strings through code.
2. The public label prefix is read from config (default `nexuspaas.io/`); the
   configured prefix is what gets injected onto workloads.
3. Legacy environment variables keep documented aliases and deprecation rules.
4. Rebuild user-facing surfaces; the engineering repo name stays NexusPaaS.

## Internal / Upstream Names

| Surface | Allowed Name |
|---|---|
| Open-source repository | NexusPaaS |
| Engineering ADRs | NexusPaaS or the configured product brand, depending on context |
| Go module path | May remain NexusPaaS |
| Legacy environment variables | May remain, but must be deprecated with documented aliases |
| Internal migration notes | May mention NexusPaaS |

## Acceptance Criteria

These are reframed from "must display CSCC" to "brand is config-driven from a
single source; default is NexusPaaS; a downstream build rebrands to the
configured name (e.g. `CSCC AI Platform`) with no code change". Each original
requirement's intent is preserved.

| ID | Acceptance Criteria |
|---|---|
| NAME-01 | End-user UI must display the configured product title (default `NexusPaaS`), sourced from brand config, not hardcoded. |
| NAME-02 | CLI binary name is the configured value (default `nexus`). |
| NAME-03 | Public CLI examples use the configured binary name; no example hardcodes a brand that cannot be swapped via config. |
| NAME-04 | User-facing Kubernetes labels use the configured public label prefix, defaulting to `nexuspaas.io/*`. |
| NAME-05 | API examples must use `/api/v1` and the configured public domains; no user-facing example should require a hardcoded brand name. |
| NAME-06 | Open-source README may state that NexusPaaS powers the deployed product; deployment docs must use the configured brand naming. |
| NAME-07 | Legacy `NEXUSPAAS_*` env vars, if kept, must have documented aliases and deprecation rules; a rebrand may add brand-prefixed aliases. |
| NAME-08 | Audit logs may store both `upstream_project="NexusPaaS"` and `product="<configured brand>"`, but exported user reports must use the configured brand naming. |
