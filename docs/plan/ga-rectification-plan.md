# NexusPaas GA 整改計畫與實作追蹤

> Status source of truth. 每項標註 **實作狀態**，證據綁 file:line / test 名稱。
> 三-agent workflow fallback：本輪 Claude 同時擔任 Plan / Reviewer / Code Agent
> （Codex quota 不可用），依 `AGENTS.md` 規定記錄 fallback，不跳過 review。

## 環境限制（決定哪些 checkbox 本機無法打勾）

- `go test` **可執行**（本機 go1.26.0，module cache warm，無需下載 toolchain）。
  → 純程式碼 + 單元/整合測試的項目可以真正完成並驗證。
- **無外部 staging 基礎設施**（Harbor、K8s cluster、Postgres PITR、Dex、OTEL、
  object store）。→ Gate G / 需 live evidence / 需 build backend / benchmark 的項目
  **本環境無法打勾**，硬打就是造假（違反本計畫第 7 節）。這些標為 `INFRA-BLOCKED`。

實作狀態圖例：
- `DONE` — 程式碼 + 測試完成且 `go test` 通過。
- `CODE` — 程式碼完成，但需 live 環境才能端到端驗證。
- `INFRA-BLOCKED` — 需外部基礎設施 / build backend / benchmark，本環境無法完成。
- `TODO` — 尚未動工。

---

## 本輪已完成並驗證（go test 通過）

- [x] **P0-2** build idempotency fingerprint 納入 source content
  （`imageregistry/handler.go` — dockerfile sha256 / context / storage_path / build_args）。
  測試：`TestImageBuildIdempotencyKeyRejectsDifferentSource`、
  `TestImageBuildIdempotencyKeyFingerprintsStorageSource`。
- [x] **P0-7 (a)** revocation store 錯誤在 production **fail-closed**
  （`platform/auth.go tokenRevoked`）。測試：`TestTokenRevokedFailClosedByEnvironment`。
- [x] **P0-7 (b) / P0-9(config)** production 缺 image-check / provenance / OTEL endpoint
  → 啟動失敗（`platform/config.go validateProductionSupplyChain`）。
  測試：`TestProductionSupplyChainFailClosed`。
- [x] **P0-9(manifest)** `production-beta/backend-units.yaml` OTEL endpoint 由 `""`
  改為真 collector DNS，並補 `K8S_IMAGE_CHECK_ENABLED` / `IMAGE_PUBLISH_REQUIRE_PROVENANCE`。
- [x] **P1-2** `backend-units.yaml` 8 unit `replicas 1→2`、HPA `minReplicas 1→2`
  （PDB minAvailable=1 現在真正可承受 1 次 voluntary disruption）。
- [x] **P0-3 (app 層)** workload dispatch 拒絕 tenant 提交的高風險資源與 pod 欄位
  （`workload/dispatcher.go`）：
  - kind deny-list：Namespace / ServiceAccount / RBAC / Ingress / PersistentVolume /
    webhook / CRD / ResourceQuota / LimitRange / NetworkPolicy（Secret 原本已擋）。
  - pod 欄位 deny-list：hostNetwork / hostPID / hostIPC / hostPath / hostPort /
    privileged / added capabilities，覆蓋一般、user-VCJob、synthesized-VCJob 三條路徑。
  測試：`TestDispatchResourcesRejectPrivilegedKinds`、`TestDispatchManifestsRejectUnsafePodFields`。
  註：cluster 側 admission（Kyverno/PSA/namespace bootstrap quota+default-deny NetworkPolicy）
  需 live cluster，屬 `INFRA-BLOCKED`。
- [x] **P0-1 (archive 安全層)** build-context archive 驗證器（`imageregistry/buildcontext.go`）：
  tar.gz / zip 自動偵測，擋 path traversal / absolute path / symlink / hardlink /
  device·fifo·socket / zip bomb（壓縮 + 解壓上限）/ file-count / path depth / path length，
  並產生 order-independent、跨格式一致的 content digest。已 wire 進 build create
  路徑（interim base64 `context_archive`）：惡意 archive → 400、digest 進 idempotency
  fingerprint（接 P0-2）並寫入 `source_digest`。
  測試：`TestValidateBuildContextArchive*`（5 支）、`TestImageBuild*ContextArchive*`（3 支）。
  註：multipart 上傳 + object-store staging + BuildKit/Tekton/Harbor push/SBOM/scan/sign
  仍屬 `INFRA-BLOCKED`（需 registry/cluster）。
- [x] **P0-4 (RBAC manifest)** compute-control-plane 專用 ServiceAccount +
  wildcard-free least-privilege ClusterRole + ClusterRoleBinding
  （`backend-units.yaml`）：ClusterRole 精確列舉 cluster client 實際使用的
  resources/verbs（namespaces/pods/pods-log/configmaps/services/secrets(create-only)/
  nodes/deployments/statefulsets/jobs/cronjobs/ingresses/priorityclasses/
  volcano vcjobs·podgroups·queues/DRA resourceclaims/longhorn volumes）。
  只有 compute-control-plane `automountServiceAccountToken: true` + serviceAccountName，
  其餘 7 unit 維持 false。測試：`TestProductionBetaComputeControlPlaneRBAC`
  （含 wildcard-free + 唯一 automount 檢查）。
  註：live 「能在 staging 建 workload / 越權被拒」驗證需 real cluster，屬 `INFRA-BLOCKED`。

驗證範圍：`go test ./internal/platform/ ./internal/services/{imageregistry,workload,schedulerquota,...} ./internal/contracts/`
全數 `ok`；`go build ./...` 通過；`go vet` clean；`gofmt` clean。
e2e/live 測試需 `TEST_DATABASE_URL` 等外部依賴，屬 `INFRA-BLOCKED`，本環境無法執行。

## P0 Blocking Changes — 實作追蹤

| 編號 | 問題 | 證據 | 狀態 | 本輪產出 |
|---|---|---|---|---|
| P0-1 | 無真正 build pipeline | `imageregistry/handler.go:675-685` queued metadata；`helpers.go:345` comment | 部分 | archive 安全驗證層 + immutable staging 契約可做（`CODE`）；BuildKit/Tekton/Harbor push/SBOM/scan/sign = `INFRA-BLOCKED` |
| P0-2 | idempotency fingerprint 不含 source | `handler.go:768` 8 欄無 source | **DONE** | fingerprint 納入 dockerfile sha256 / context / storage_path / build_args；3 source 測試 |
| P0-3 | workload 可提交 raw K8s / unsafe 欄位 | `platform/cluster/apply.go` 等 | 部分 | 使用者 spec 拒 raw object + unsafe 欄位 deny-list（`CODE`+test）；Kyverno/PSA e2e = `INFRA-BLOCKED` |
| P0-4 | compute-control-plane 無 K8s RBAC | `backend-units.yaml` 全 unit SA token=false | `CODE` | manifest 加專用 SA + least-privilege Role/RoleBinding；live 建 workload = `INFRA-BLOCKED` |
| P0-5 | 核心資料仍 generic JSON | typed 僅 identity/notification | 進行中（逐 domain） | image_builds domain 已 typed（見下）；其餘 domain 仍待逐一切；每片需 real Postgres 做 constraint/rollback drill = `INFRA-BLOCKED` |
| P0-6 | GA 證據僅 local kind | blocker ledger | `INFRA-BLOCKED` | 需真 staging |
| P0-7 | config 未全 fail-closed；revocation fail-open | `platform/auth.go:103-111` | **DONE** | revocation store 錯誤 fail-closed（high-risk）+ prod config 強制檢查 + 測試 |
| P0-8 | 無 backup/restore drill | migration runner 無 down | `INFRA-BLOCKED` | 需真 Postgres |
| P0-9 | observability 未 live | OTEL endpoint 可空 | 部分 | prod config 強制 OTEL endpoint 非空（`CODE`）；live dashboard = `INFRA-BLOCKED` |

## P0-5 已完成並驗證（一個 domain slice）

- [x] **image_builds → typed table**（`migrations/image-registry-service/0002_image_builds_typed.sql`
  + `platform/store_postgres_imageregistry.go`）：typed `image_build_jobs` table
  （PK、promoted 欄位 project_id/image_reference/build_type/status/requested_by/source_digest、
  status not-blank CHECK、project_id + status index、payload JSONB 保留全記錄），
  從 `platform_records` backfill（保留舊列可回滾），註冊進 `typedPostgresResourceFor`
  自動路由 CRUD。跟 P0-1/P0-2 的 source_digest 對接。
  測試：`TestTypedPostgresResourceRoutesImageBuildJobs`、
  `TestImageBuildInsertColumnsPromoteQueryFields`、
  `TestPostgresStoreRoutesImageBuildJobsToOwnedTable`（fake-DB 驗證路由到 typed table 非 platform_records）；
  migration 結構驗證（migrate_test）全綠。
  註：constraint 實際強制 / rollback drill 需 real Postgres = `INFRA-BLOCKED`；其餘 core domain
  （org-project/authz/workload/scheduler/storage/usage/audit）尚待逐一 typed 化。

## P1 已完成並驗證

- [x] **P1-7 分散式 rate limit（`platform/ratelimit.go` + `middleware.go`）**：
  - route class 分類（build / transfer / workload / auth / default）+ per-class 較嚴 quota
    （build 30、transfer/workload 60、auth 20 /min；default 沿用 limiter 設定值）。
  - key 綁 class + principal（authed user，否則 client IP），各 class 各自預算，
    互不干擾。Redis-backed 時多副本一致（`AllowWithin` 走同一 Lua INCR+PEXPIRE）。
  - rejection 有 audit（slog.Warn）+ metric（`rate_limit_rejected_total{class=...}`）。
  測試：`TestRateLimitClassification`、`TestSpecialRateLimitPolicyIsTighterThanDefault`、
  `TestRateLimiterAllowWithinIsPerCallLimitAndPerKey`、`TestRateLimitRejectionEmitsClassMetric`；
  既有 rate-limit 測試全數維持綠燈。
  註：per-org quota 需 request 帶 org context，暫以 per-user/per-class 覆蓋。

## P1 / P2 其餘

見本 repo 前次分析（Executive Summary → 第 7 節）。本輪碰觸：
- P1-2 HA：`backend-units.yaml` stateless unit replicas 2 / HPA min 2 / PDB minAvailable 1→保持可承受單點 drain（`CODE`）。
- 其餘 P1/P2 屬 60/90 天，或需 infra。

---

## 本輪不做（誠實聲明）

- ❌ 不用 local kind 冒充 production evidence。
- ❌ 不把 queued metadata 說成 build pipeline —— P0-1 build backend 明確標 `INFRA-BLOCKED`。
- ❌ 不把 `rsync -a`/`cp -a` 說成 HPC optimization。
- ❌ 不把 replicas=1 說成 HA。
- ❌ 不硬打需要 live 環境的 checkbox。

詳見 commit diff 與各檔測試。
