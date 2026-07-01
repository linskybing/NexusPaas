# Kind Live E2E — V1 Launch-Blocker Drills (kind-tier evidence)

> **KIND LOCAL — NOT EXTERNAL GA PROOF.** Per `docs/agents/workflow.md`,
> single-cluster/local (kind) evidence must not be described as external GA
> proof. This report proves the deploy / migration / rollback / supply-chain
> **machinery executes on a real Kubernetes cluster** (kind), upgrading prior
> render-only/static evidence. The **external registry host** and **external
> staging cluster** rows remain **Open**.

- Run ID: `20260701061628`  •  Cluster: `kind-nexuspaas-kind-e2e`  •  Namespace: `nexuspaas`
- Baseline image: `nexuspaas-backend:v0.1.0`  •  Candidate image: `nexuspaas-backend:v0.1.1`
- kind: `kind v0.32.0 go1.26.3 linux/amd64`  •  kubectl: `v1.36.2`

## 1. Image supply chain (kind-tier)

| step | tool | status | detail |
| --- | --- | --- | --- |
| image-build | docker-buildkit | pass | sha256:72966a9aa7fd3809c676a54c5d99ac73d80017ff76040e9b5e0ac95f3084fc3b |
| sbom | syft | pass | 328139 bytes |
| scan | trivy | pass | HIGH+CRITICAL=0 |
| sign | cosign | pass | local keypair generated (offline sign requires a registry ref) |

## 2. Secret presence (names only; no values)

| secret | present |
| --- | --- |
| postgres-password | yes |
| minio-credentials | yes |
| dex-password | yes |
| coturn-runtime-secret | yes |
| platform-gateway-runtime-secret | yes |
| iam-unit-runtime-secret | yes |
| tenant-unit-runtime-secret | yes |
| collaboration-unit-runtime-secret | yes |
| platform-io-unit-runtime-secret | yes |
| usage-observability-runtime-secret | yes |
| compute-api-runtime-secret | yes |
| compute-control-plane-runtime-secret | yes |

## 3. Live DB migration apply / validate / idempotency (real Postgres)

| task | job | status |
| --- | --- | --- |
| apply-migrations | kind-apply-migrations-20260701061628-1 | complete |
| validate-migrations | kind-validate-migrations-20260701061628-2 | complete |
| apply-migrations | kind-apply-migrations-20260701061628-3 | complete |

DB **schema down-migration is not an app capability** (forward-only migrations
with dirty-tracking); DB rollback = restore-from-backup, out of kind scope and
still Open. Deployment-level rollback is proven in section 5.

## 4. 8-unit rollout + smoke (/healthz /readyz /metrics + 15-service registry)

| phase | unit | endpoint | code |
| --- | --- | --- | --- |
| baseline-v0.1.0 | platform-gateway | /healthz | 200 |
| baseline-v0.1.0 | platform-gateway | /readyz | 200 |
| baseline-v0.1.0 | platform-gateway | /metrics | 200 |
| baseline-v0.1.0 | platform-gateway | /openapi.json | 200 |
| baseline-v0.1.0 | iam-unit | /healthz | 200 |
| baseline-v0.1.0 | iam-unit | /readyz | 200 |
| baseline-v0.1.0 | iam-unit | /metrics | 200 |
| baseline-v0.1.0 | iam-unit | /openapi.json | 200 |
| baseline-v0.1.0 | tenant-unit | /healthz | 200 |
| baseline-v0.1.0 | tenant-unit | /readyz | 200 |
| baseline-v0.1.0 | tenant-unit | /metrics | 200 |
| baseline-v0.1.0 | tenant-unit | /openapi.json | 200 |
| baseline-v0.1.0 | collaboration-unit | /healthz | 200 |
| baseline-v0.1.0 | collaboration-unit | /readyz | 200 |
| baseline-v0.1.0 | collaboration-unit | /metrics | 200 |
| baseline-v0.1.0 | collaboration-unit | /openapi.json | 200 |
| baseline-v0.1.0 | platform-io-unit | /healthz | 200 |
| baseline-v0.1.0 | platform-io-unit | /readyz | 200 |
| baseline-v0.1.0 | platform-io-unit | /metrics | 200 |
| baseline-v0.1.0 | platform-io-unit | /openapi.json | 200 |
| baseline-v0.1.0 | usage-observability | /healthz | 200 |
| baseline-v0.1.0 | usage-observability | /readyz | 200 |
| baseline-v0.1.0 | usage-observability | /metrics | 200 |
| baseline-v0.1.0 | usage-observability | /openapi.json | 200 |
| baseline-v0.1.0 | compute-api | /healthz | 200 |
| baseline-v0.1.0 | compute-api | /readyz | 200 |
| baseline-v0.1.0 | compute-api | /metrics | 200 |
| baseline-v0.1.0 | compute-api | /openapi.json | 200 |
| baseline-v0.1.0 | compute-control-plane | /healthz | 200 |
| baseline-v0.1.0 | compute-control-plane | /readyz | 200 |
| baseline-v0.1.0 | compute-control-plane | /metrics | 200 |
| baseline-v0.1.0 | compute-control-plane | /openapi.json | 200 |
| baseline-v0.1.0 | ALL-8-UNITS | /service-registry (union) | 15-of-15 |
| candidate-v0.1.1 | platform-gateway | /healthz | 200 |
| candidate-v0.1.1 | platform-gateway | /readyz | 200 |
| candidate-v0.1.1 | platform-gateway | /metrics | 200 |
| candidate-v0.1.1 | platform-gateway | /openapi.json | 200 |
| candidate-v0.1.1 | iam-unit | /healthz | 200 |
| candidate-v0.1.1 | iam-unit | /readyz | 200 |
| candidate-v0.1.1 | iam-unit | /metrics | 200 |
| candidate-v0.1.1 | iam-unit | /openapi.json | 200 |
| candidate-v0.1.1 | tenant-unit | /healthz | 200 |
| candidate-v0.1.1 | tenant-unit | /readyz | 200 |
| candidate-v0.1.1 | tenant-unit | /metrics | 200 |
| candidate-v0.1.1 | tenant-unit | /openapi.json | 200 |
| candidate-v0.1.1 | collaboration-unit | /healthz | 200 |
| candidate-v0.1.1 | collaboration-unit | /readyz | 200 |
| candidate-v0.1.1 | collaboration-unit | /metrics | 200 |
| candidate-v0.1.1 | collaboration-unit | /openapi.json | 200 |
| candidate-v0.1.1 | platform-io-unit | /healthz | 200 |
| candidate-v0.1.1 | platform-io-unit | /readyz | 200 |
| candidate-v0.1.1 | platform-io-unit | /metrics | 200 |
| candidate-v0.1.1 | platform-io-unit | /openapi.json | 200 |
| candidate-v0.1.1 | usage-observability | /healthz | 200 |
| candidate-v0.1.1 | usage-observability | /readyz | 200 |
| candidate-v0.1.1 | usage-observability | /metrics | 200 |
| candidate-v0.1.1 | usage-observability | /openapi.json | 200 |
| candidate-v0.1.1 | compute-api | /healthz | 200 |
| candidate-v0.1.1 | compute-api | /readyz | 200 |
| candidate-v0.1.1 | compute-api | /metrics | 200 |
| candidate-v0.1.1 | compute-api | /openapi.json | 200 |
| candidate-v0.1.1 | compute-control-plane | /healthz | 200 |
| candidate-v0.1.1 | compute-control-plane | /readyz | 200 |
| candidate-v0.1.1 | compute-control-plane | /metrics | 200 |
| candidate-v0.1.1 | compute-control-plane | /openapi.json | 200 |
| candidate-v0.1.1 | ALL-8-UNITS | /service-registry (union) | 15-of-15 |
| rollback | platform-gateway | /healthz | 200 |
| rollback | platform-gateway | /readyz | 200 |
| rollback | platform-gateway | /metrics | 200 |
| rollback | platform-gateway | /openapi.json | 200 |
| redeploy | platform-gateway | /healthz | 200 |
| redeploy | platform-gateway | /readyz | 200 |
| redeploy | platform-gateway | /metrics | 200 |
| redeploy | platform-gateway | /openapi.json | 200 |
| rollback | iam-unit | /healthz | 200 |
| rollback | iam-unit | /readyz | 200 |
| rollback | iam-unit | /metrics | 200 |
| rollback | iam-unit | /openapi.json | 200 |
| redeploy | iam-unit | /healthz | 200 |
| redeploy | iam-unit | /readyz | 200 |
| redeploy | iam-unit | /metrics | 200 |
| redeploy | iam-unit | /openapi.json | 200 |
| rollback | tenant-unit | /healthz | 200 |
| rollback | tenant-unit | /readyz | 200 |
| rollback | tenant-unit | /metrics | 200 |
| rollback | tenant-unit | /openapi.json | 200 |
| redeploy | tenant-unit | /healthz | 200 |
| redeploy | tenant-unit | /readyz | 200 |
| redeploy | tenant-unit | /metrics | 200 |
| redeploy | tenant-unit | /openapi.json | 200 |
| rollback | collaboration-unit | /healthz | 200 |
| rollback | collaboration-unit | /readyz | 200 |
| rollback | collaboration-unit | /metrics | 200 |
| rollback | collaboration-unit | /openapi.json | 200 |
| redeploy | collaboration-unit | /healthz | 200 |
| redeploy | collaboration-unit | /readyz | 200 |
| redeploy | collaboration-unit | /metrics | 200 |
| redeploy | collaboration-unit | /openapi.json | 200 |
| rollback | platform-io-unit | /healthz | 200 |
| rollback | platform-io-unit | /readyz | 200 |
| rollback | platform-io-unit | /metrics | 200 |
| rollback | platform-io-unit | /openapi.json | 200 |
| redeploy | platform-io-unit | /healthz | 200 |
| redeploy | platform-io-unit | /readyz | 200 |
| redeploy | platform-io-unit | /metrics | 200 |
| redeploy | platform-io-unit | /openapi.json | 200 |
| rollback | usage-observability | /healthz | 200 |
| rollback | usage-observability | /readyz | 200 |
| rollback | usage-observability | /metrics | 200 |
| rollback | usage-observability | /openapi.json | 200 |
| redeploy | usage-observability | /healthz | 200 |
| redeploy | usage-observability | /readyz | 200 |
| redeploy | usage-observability | /metrics | 200 |
| redeploy | usage-observability | /openapi.json | 200 |
| rollback | compute-api | /healthz | 200 |
| rollback | compute-api | /readyz | 200 |
| rollback | compute-api | /metrics | 200 |
| rollback | compute-api | /openapi.json | 200 |
| redeploy | compute-api | /healthz | 200 |
| redeploy | compute-api | /readyz | 200 |
| redeploy | compute-api | /metrics | 200 |
| redeploy | compute-api | /openapi.json | 200 |
| rollback | compute-control-plane | /healthz | 200 |
| rollback | compute-control-plane | /readyz | 200 |
| rollback | compute-control-plane | /metrics | 200 |
| rollback | compute-control-plane | /openapi.json | 200 |
| redeploy | compute-control-plane | /healthz | 200 |
| redeploy | compute-control-plane | /readyz | 200 |
| redeploy | compute-control-plane | /metrics | 200 |
| redeploy | compute-control-plane | /openapi.json | 200 |

## 5. Per-unit previous-image rollback + redeploy

| unit | phase | image | status |
| --- | --- | --- | --- |
| platform-gateway | rollback | nexuspaas-backend:v0.1.0 | complete |
| platform-gateway | redeploy | nexuspaas-backend:v0.1.1 | complete |
| iam-unit | rollback | nexuspaas-backend:v0.1.0 | complete |
| iam-unit | redeploy | nexuspaas-backend:v0.1.1 | complete |
| tenant-unit | rollback | nexuspaas-backend:v0.1.0 | complete |
| tenant-unit | redeploy | nexuspaas-backend:v0.1.1 | complete |
| collaboration-unit | rollback | nexuspaas-backend:v0.1.0 | complete |
| collaboration-unit | redeploy | nexuspaas-backend:v0.1.1 | complete |
| platform-io-unit | rollback | nexuspaas-backend:v0.1.0 | complete |
| platform-io-unit | redeploy | nexuspaas-backend:v0.1.1 | complete |
| usage-observability | rollback | nexuspaas-backend:v0.1.0 | complete |
| usage-observability | redeploy | nexuspaas-backend:v0.1.1 | complete |
| compute-api | rollback | nexuspaas-backend:v0.1.0 | complete |
| compute-api | redeploy | nexuspaas-backend:v0.1.1 | complete |
| compute-control-plane | rollback | nexuspaas-backend:v0.1.0 | complete |
| compute-control-plane | redeploy | nexuspaas-backend:v0.1.1 | complete |

## 6. Local-registry promote / rollback (kind-tier; NOT an external registry)

| step | ref | status |
| --- | --- | --- |
| push-previous | 127.0.0.1:5000/nexuspaas-backend:v0.1.0 | ok |
| promote-candidate | 127.0.0.1:5000/nexuspaas-backend:v0.1.1 | ok |
| rollback-pull-previous | 127.0.0.1:5000/nexuspaas-backend:v0.1.0 | ok |
| digests | prev=n/a candidate=n/a | ok |

## Residual (stays Open — external only)

- External registry host promotion/rollback (Harbor as the real external GA registry).
- External staging cluster deploy/secret provenance/DR (off-cluster, HA).
- DB schema rollback via restore-from-backup on external staging.
- Product image-build dispatch feature (Tekton/BuildKit, source upload/hash) — not implemented.
- Full live PERF/MON soak, browser WebRTC, real GPU.

_Raw artifacts (TSVs, SBOM, scan, logs): `/tmp/nexuspaas-kind-live-e2e/20260701061628`_
