# Archive/Image Build and HPC Storage Audit

Date: 2026-06-30

This audit cross-checks documentation, handlers, service code, fixtures, tests,
Kubernetes manifests, and deployment/security configuration. It deliberately
does not treat `references/CSCC_AI_Platform_Backend/**` as product
implementation.

## A. 總結判斷

1. 文件與程式碼有明顯偏離，但主要偏離已被 `docs/acceptance/image-build.md`
   和 `docs/acceptance/gap-analysis.md` 局部承認：image build 是目標/AC
   描述加上 local/static evidence，不是完整 production pipeline。
2. `POST /api/v1/images/build`、`/from-storage`、`/dockerfile` 都已註冊，但
   handler 只進入同一個 `createBuild`，解 JSON body 後建立 `queued` metadata。
3. 目前沒有 tar.gz/zip multipart upload、archive parsing、Dockerfile context
   unpack、object-storage build-context upload、BuildKit/Tekton job dispatch、
   Harbor push、SBOM/scan/sign/attestation 實際狀態轉移。
4. Dockerfile/context/storage_path/build_args 在 API fixture 中是 optional
   contract 欄位，但 `createBuild` 沒有保存、hash、驗證或派工使用這些 source。
5. image build idempotency fingerprint 只含 requested id、user、build type、
   project、image reference、CPU、memory、timeout，沒有包含 Dockerfile 內容、
   archive/context hash、storage object/path、build args、source revision/checksum。
6. HPC storage 有設計與 control-plane metadata：storage profiles、HPC
   StorageClass manifests、DataPlanePlan、CacheBinding、BenchmarkRecord、
   FastTransfer record 和 mover Job。
7. HPC data-plane 目前不是完整最佳化：stage-in 是 `cp -a` initContainer，
   FastTransfer mover 是單一 `rsync -a --delete` Job，只送 1%/100% progress。
8. 目前最接近成熟度是 **HPC storage planning layer**，外加少量 basic
   data-plane runtime evidence；不是 production-grade HPC storage optimization。

## B. 壓縮檔 / image build 實作檢查

| 檢查項目 | 是否有文件宣稱 | 是否有程式碼實作 | 證據 | 結論 |
|---|---|---|---|---|
| tar.gz upload | 有。`image-build.md` 把 `tar.gz / zip upload` 列為 supported source。 | 沒有。image build handler 沒有 `ParseMultipartForm`/`FormFile`/archive parser。 | `docs/acceptance/image-build.md:13-23`; `backend/internal/services/imageregistry/handler.go:618-626` 只 `DecodeMapWithError`。 | 文件超前實作；不是已支援。 |
| zip upload | 有。與 tar.gz 同列。 | 沒有。沒有 zip build-context extraction 或 zip bomb 防護。 | 同上；repo 搜尋未找到 image build 使用 `archive/zip`。 | 文件超前實作。 |
| Dockerfile + build context | 有。CLI local Dockerfile + context 是 supported source，fixture 也有 `dockerfile`/`context`。 | 只有 API contract/metadata。handler 沒讀取或保存 Dockerfile/context。 | `docs/acceptance/image-build.md:17`; fixture `image-registry-dockerfile-build.json:22-47`; handler `createBuild` 只保存 build metadata at `handler.go:665-686`。 | 只有 API contract + metadata queue。 |
| from-storage build | 有。user/group/project storage 是 supported source，`/from-storage` route 存在。 | 只有 API contract/metadata。沒有 storage path permission、mount-plan lookup 或 object/PVC dispatch。 | Route at `handler.go:89`; `startStorageImageBuild` only calls `createBuild` at `handler.go:521-523`; fixture optional `storage_path` at `image-registry-storage-build.json:22-40`。 | 只有 metadata queue。 |
| archive build context | 有。Build flow 寫「upload context to object storage」。 | 沒有。沒有 image-context object upload、source digest、checksum、unpack。 | `docs/acceptance/image-build.md:31-33`; handler stores no source fields at `handler.go:665-686`。 | 文件超前實作。 |
| image build pipeline | 有。Build flow 描述 quota, upload/mount, Tekton, BuildKit, Harbor, SBOM, scan, sign, allow-list。 | 沒有 pipeline executor。只建立 `queued` row 和 `ImageBuildStarted` event。 | `docs/acceptance/image-build.md:24-41`; `handler.go:691-697` emits event and returns 202。 | 只有 metadata queue，不是 pipeline。 |
| BuildKit | 有。Acceptance IMG-010 指 rootless BuildKit through Tekton。 | 沒有 BuildKit Job/PipelineRun dispatch。 | `docs/acceptance/image-build.md:67-68`; code search shows no product BuildKit executor in imageregistry。 | 文件/AC 超前實作。 |
| Tekton | 有。Build flow 寫 Tekton PipelineRun。 | 沒有 PipelineRun manifest/controller。 | `docs/acceptance/image-build.md:31-34`; no Tekton dispatch in `imageregistry` handler/service。 | 文件/AC 超前實作。 |
| Harbor push | 有。Build flow/IMG-014 要 push Harbor。 | 沒有 build push；只有 Harbor health/catalog sync/read metadata。 | `docs/acceptance/image-build.md:35,72`; `handler.go:697` only attaches Harbor degraded metadata; catalog sync is separate Harbor artifact read path。 | live Harbor foundation/sync exists，但 build push 沒實作。 |
| SBOM | 有。Build flow 和 IMG-016 要 SBOM。 | 沒有生成。queued record 只設 `sbom_status="pending"`。 | `docs/acceptance/image-build.md:36,74`; `handler.go:676-681`。 | 只有 metadata/status placeholder。 |
| image scan | 有。Build flow 和 IMG-017 要 scan result。 | 沒有 build scan lifecycle。queued record 只設 `scan_status="pending"`；Harbor-side scan evidence 是獨立 synthetic image。 | `docs/acceptance/image-build.md:37,75`; `docs/acceptance/gap-analysis.md:246-274`; `handler.go:680`。 | 只有 metadata；不是 build pipeline scan。 |
| signing / attestation | 有。Build flow 和 IMG-018 要 signature/attestation。 | 沒有 Cosign/sign/attestation executor。queued record 只設 signature pending。 | `docs/acceptance/image-build.md:38,76`; `handler.go:678-681`。 | 只有 metadata/status placeholder。 |
| allow-list enforcement | 有。IMG-019/020/021 要 digest allow-list。 | 部分實作，但不屬於 build completion pipeline。catalog publish guard 和 scheduler submit allow-list guard 是 local/static/in-code evidence。 | `docs/acceptance/gap-analysis.md:546-574`; `handler.go:96-98` owner-read allow-list contract; scheduler admission tests exist。 | 部分 admission guard；build 成功後自動 allow-list workflow 未完成。 |
| multipart upload | 文件對 tar.gz/zip imply upload。 | image build 沒有。multipart 只出現在 media upload tests，不在 imageregistry build。 | `handler.go:623` JSON decode；no `ParseMultipartForm`/`FormFile` in image build path。 | 沒實作。 |
| Dockerfile content handling | fixture optional `dockerfile`。 | handler 不讀、不驗證、不保存、不 hash。 | `image-registry-dockerfile-build.json:22-47`; `handler.go:618-697`。 | 只有 contract 欄位。 |
| build context handling | fixture optional `context`。 | handler 不解析、不上傳、不 hash。 | `image-registry-context-build.json:22-45`; `handler.go:665-686`。 | 只有 contract 欄位。 |
| object storage upload | Build flow 宣稱 upload context to object storage。 | image build 沒有 object-store call。 | `docs/acceptance/image-build.md:31-33`; image build code only store record。 | 沒實作。 |
| source digest / checksum | IMG-025 事件要 source type/resources/image digest/scan/allow-list decision；from-storage fixture有 source path。 | build idempotency/hash 不含 source checksum，record 初始 `image_digest=""`。 | `handler.go:647-656`, `handler.go:768-777`。 | 缺 source provenance。 |
| unpack / extract | archive build implies extraction。 | 沒有。 | no tar/zip extractor in product image build path。 | 沒實作。 |
| path traversal 防護 | archive extraction 必須具備。 | 沒有，因為沒有 archive extraction pipeline。 | no canonical archive entry validation in imageregistry。 | 啟用 archive 前的 P0/P1 安全缺口。 |
| symlink / hardlink 防護 | archive extraction 必須具備。 | 沒有。 | no tar header link policy in imageregistry。 | 沒實作。 |
| zip bomb 防護 | zip upload 必須具備。 | 沒有。 | no compressed/uncompressed ratio, max files, max entry size checks。 | 沒實作。 |
| max archive size / max file count | archive upload 必須具備。 | 沒有。 | no image-build upload limiter or archive counter。 | 沒實作。 |
| build logs | 有 IMG-011。 | 部分。GET logs returns stored string with redaction; no live executor streaming/tailing。 | `handler.go:543-558`; redaction at `handler.go:561-583`; gap says no live logs at `gap.md:641-646`。 | local/static log response only。 |
| cancel build | 有 IMG-012。 | 部分。DELETE sets metadata status `cancelled`; no executor/K8s termination。 | `handler.go:586-615`; gap says no live executor cancellation at `gap.md:591-600`。 | metadata cancellation only。 |
| build status update/result digest | 有 IMG-015/017/018/019/025。 | queued create only; no completion controller writing digest or final scan/sign/SBOM states。 | `handler.go:675-681`。 | 只有 pending metadata。 |

## C. 文件與程式碼偏離清單

| 偏離項目 | 文件說法 | 程式碼實際狀況 | 風險 | 建議 |
|---|---|---|---|---|
| Supported Build Sources 用詞暗示已支援 | `image-build.md` 列出 local Dockerfile+context、tar.gz/zip、user/group/project storage。 | 只有三個 REST route + fixtures；handler 不處理 source content。 | 使用者會以為可上傳 archive 或從 storage build，實際只得到 queued metadata。 | 把「Supported」改成「Target/Planned」或在同段加 Current implementation status。 |
| Build Flow 描述完整 pipeline | 文件列 Tekton/BuildKit/Harbor/SBOM/scan/sign/allow-list。 | 沒有 executor；`createBuild` 只建 record/event。 | production readiness 誤導；queued build 永遠不會產出 image digest。 | 在文件中標明 flow 是 GA target；新增 live executor 前不要宣稱完成。 |
| Dockerfile fixture 讓人以為 handler 使用 Dockerfile | fixture request example 有 `dockerfile`、`context`、`build_args`。 | handler 忽略這些欄位。 | retry/provenance 和重現性錯誤；不同 Dockerfile 可被同一 idempotency key 視為同一 request。 | 實作前先在 create API 拒絕 source 欄位或保存/hash source metadata。 |
| from-storage fixture 讓人以為有 storage permission | fixture optional `storage_path`。 | handler 不驗證 storage path、Project storage permission 或 mount-plan。 | 越權/錯誤語意風險，一旦 executor 補上容易漏做 trust-boundary。 | from-storage 必須接 storage service mount-plan/permission owner-read。 |
| idempotency fingerprint 缺 build source | 文件/fixtures允許 dockerfile/context/storage_path/build_args。 | fingerprint 只含 request id/user/build type/project/image ref/resources/time。 | 同一 Idempotency-Key 重送不同 Dockerfile/context 可能回放舊 build；provenance 不可靠。 | fingerprint 加 source type、Dockerfile hash、archive/context hash、storage object/path/id、build args、revision、checksum。 |
| logs/cancel 看似 build lifecycle | AC 要 logs/cancel release quota。 | logs 是 stored string；cancel 改 metadata `cancelled`。 | 使用者以為 live executor 被終止，實際資源不會被釋放，因為沒有 executor。 | 實作 executor controller 後再關閉 IMG-011/012/013。 |
| SBOM/scan/sign 狀態 | 文件列為 acceptance criteria。 | 只有 `pending` 欄位和 publish presence guard；沒有生成/掃描/簽章。 | supply-chain policy 可能被當作完成。 | 以 Syft/Trivy/Cosign 或等價工具補 runtime transition。 |
| Harbor evidence | gap/README 有 Harbor foundation 和 synthetic scan/sync evidence。 | build API 不 push Harbor；catalog sync 是 artifact read/sync，不是 build output。 | Harbor exists 被誤解為 image build pipeline ready。 | 把 Harbor foundation/sync 和 build push 分開標示。 |
| HPC storage profile | docs/manifests 有 local NVMe/CephFS/Longhorn/MinIO 分層。 | storage profile 和 manifests 有，但 runtime 只證明 kind/default PVC 和 tiny file。 | storage profile 被誤解為已具 HPC performance。 | 文件明確區分 profile design 與 data-plane optimization。 |
| FastTransfer progress/checksum/resume | API/fixtures 有 bytes/checksum/resume metadata。 | mover 只 `rsync -a --delete`，只 callback 1%/100%，不計 bytes/checksum/resume。 | UI/API 顯示可能不代表真實進度或完整性。 | mover 必須計算 bytes、throughput、checksum，並實作 resume token。 |
| BenchmarkRecord | benchmark record API 存在。 | 只存 metadata；沒有 fio/IOR/mdtest runner。 | benchmark 被誤認為性能測試完成。 | 加 runner、artifact、baseline、profile feedback loop。 |

## D. HPC storage 檢查

| 項目 | 是否存在 | 是設計 / metadata / manifest / data-plane 實作 | 評價 |
|---|---|---|---|
| local NVMe scratch | 有。`local-nvme-scratch` profile and StorageClass。 | Profile + manifest + scratch PVC creation。 | planning/control-plane；未證明真 local NVMe PV binding/perf。 |
| node-local cache | 有 CacheBinding 概念。 | metadata + DataPlanePlan cache-hit flag。 | 沒有 live cache residency、eviction、prewarm。 |
| CephFS RWX authority tier | 有。`cephfs-rwx-authority` profile/StorageClass。 | manifest + checkpoint target profile。 | manifest/design；未證明 live CephFS runtime 或 stripe tuning。 |
| Longhorn RWX compatibility tier | 有。profile/StorageClass + Longhorn RWX health worker。 | manifest + health summary/reconcile code。 | compatibility/health layer；不是 HPC optimization。 |
| MinIO / S3 artifact tier | 有 `minio-artifact` profile and MinIO deployment/docs。 | metadata/profile + object restore drill elsewhere。 | artifact tier concept exists；image build context upload not implemented。 |
| checkpoint staging | 有 checkpoint local path/flush target env。 | DataPlanePlan injects env only。 | metadata/env injection；no real async flush。 |
| async checkpoint flush | write policy default is `local-first-async-flush`。 | string/env only。 | Not implemented as mover/sidecar/controller。 |
| dataset stage-in | 有 DataPlane stage-in operations。 | workload initContainer `cp -a` from stage PVC to scratch。 | basic copy only；not optimized。 |
| job-local scratch mount | 有。scratch PVC is created and mounted。 | workload dispatcher creates PVC and injects volume mount。 | partial runtime implementation；no quota-aware sizing/perf proof。 |
| cache binding | 有 CRUD and plan cache-hit marking。 | metadata/control-plane。 | no live cache lifecycle。 |
| data plane profile | 有 DataPlanePlan request/response。 | internal service contract + storage profile lookup。 | planning layer implemented。 |
| storage profile | 有 default profiles and manifest drift test。 | metadata + K8s manifests。 | good control-plane inventory；not proof of storage performance。 |
| benchmark record | 有 create/list event。 | metadata only。 | no fio/IOR/mdtest execution。 |
| transfer / mover service | 有 FastTransfer and k8s-control mover Job。 | single Kubernetes Job running `rsync -a --delete`。 | basic data sync, not HPC optimized transfer。 |
| parallel copy / multi-worker | 沒有。 | none。 | not implemented。 |
| rsync parallelization / fpsync / msrsync | 沒有。 | mover allows only `rsync` and runs one command。 | not implemented。 |
| rclone multipart / object multipart upload | 沒有。 | none in FastTransfer/image build path。 | not implemented。 |
| tar streaming | 沒有。 | none。 | not implemented。 |
| large-file / small-file strategies | 沒有。 | none。 | not implemented。 |
| checksum validation | 部分 metadata。 | storage progress can store checksum; mover does not calculate or verify。 | metadata only。 |
| resume token | 部分 metadata。 | API/state can store token; mover does not resume。 | metadata only。 |
| bytes copied tracking | 部分 metadata。 | progress state accepts bytes; mover only posts progress pct 1/100。 | not accurate byte accounting。 |
| throughput metrics / ETA / progress | 粗略 progress only。 | no throughput/ETA; two callbacks。 | insufficient。 |
| node affinity / data locality scheduling | 部分 design。 | profile has `node_selector`/`topology_policy`; dispatcher does not prove profile-driven scheduling/locality。 | planning metadata。 |
| metadata storm mitigation / small-file packing | 沒有。 | none。 | not implemented。 |
| CephFS stripe layout tuning | 沒有。 | manifest only `noatime`。 | not implemented。 |
| Lustre / GPFS / BeeGFS support | 沒有。 | none。 | not implemented。 |
| fio / IOR / mdtest / checkpoint benchmark | 沒有 runtime runner。 | BenchmarkRecord metadata only。 | not implemented。 |
| cache eviction / warmup / prefetch | 沒有。 | CacheBinding metadata only。 | not implemented。 |
| read/write performance feedback loop | 沒有。 | none。 | not implemented。 |

HPC storage 成熟度判斷：**HPC storage planning layer**。理由是
control-plane 概念和 manifests 明確存在，但 data-plane 只有 basic `cp -a`
和 single-worker `rsync -a --delete`，benchmark/cache/checkpoint 多數是
metadata 或 env wiring，尚未形成 profile-based optimized mover 或 scheduler
feedback loop。

## E. 重大風險

### P0

- Image build API 如果被 production 暴露，會接受 build request 但只建立
  queued metadata，沒有真正 build/push/scan/sign。應立刻在文件/API response/UI
  標記為 metadata-only，或在 production 關閉 build capability。
- Image build idempotency fingerprint 不含 source content。相同
  `Idempotency-Key` 可對不同 Dockerfile/context/storage source 回放舊 request，
  造成 retry semantic、reproducibility、source provenance 錯誤。

### P1

- Archive upload 安全控制完全未實作。啟用 tar.gz/zip 前必須補 path
  traversal、symlink/hardlink、zip bomb、max size、max file count、checksum。
- from-storage build 沒有 storage permission/mount-plan trust-boundary；實作
  executor 前必須先接 storage-service owner-read/permission model。
- HPC mover 不是 HPC optimized；對大量小檔、大檔、跨節點或 object store
  transfer 會缺 progress、resume、checksum、throughput 和 locality control。
- SonarCloud SECURITY 遠端清理尚未完成：本地 scanner 已通過，但
  SonarQube Cloud automatic analysis 的 `.sonarcloud.properties` 不支援
  wildcard source exclusions；需改用 SonarCloud UI Analysis Scope 或切到
  CI-based analysis 後才可關閉剩餘 remote issues。

### P2

- 文件應把 target architecture、local/static evidence、live evidence 分段。
- BenchmarkRecord/CacheBinding 可以保留，但應明確標示 metadata-only。
- FastTransfer progress API 應增加真實 bytes/throughput/ETA tests。

## F. 建議修正方向

- 文件：把 `image-build.md` 的「Supported Build Sources」改成
 「GA Target Build Sources」或加一欄 Current implementation；README 不要讓
  Harbor foundation 被理解成 image build pipeline 已完成。
- Archive upload pipeline：新增 multipart endpoint 或明確要求 JSON
  object-storage reference；先計算 upload SHA-256，再用 streaming tar/zip
  reader 做 canonical extraction；限制 compressed/uncompressed bytes、entry
  count、entry path、file mode、symlink/hardlink policy。
- Source provenance：build record 應保存 source type、Dockerfile digest、
  context archive digest、storage object ID/path/version、build args digest、
  source revision/checksum；event payload 也要帶 source digest。
- Idempotency fingerprint：至少加入 source type、Dockerfile content hash、
  context archive hash、storage path/object ID、build args、image reference、
  project ID、requested user、CPU/memory/timeout、source revision/checksum。
- BuildKit/Tekton：新增 build dispatcher/controller，建立 PipelineRun/Job，
  綁定 resource quota、timeout、cancel、log streaming、final image digest、
  Harbor push result、failure reason。
- Supply chain：build success 後執行 SBOM generation、vulnerability scan、
  signing/attestation，並以狀態機更新 `sbom_status`、`scan_status`、
  `signature_status`、`allow_list_decision`。
- Allow-list：只允許 digest-based publish；build pipeline 完成後由
  image-registry owner 寫 allow-list，scheduler 只讀 allow-list admission。
- HPC mover：把 storage profile 轉成 mover strategy，例如 local NVMe
  same-node stage-in、CephFS authority flush、object storage multipart/rclone、
  large-file vs small-file strategy、parallel workers、checksum、resume token。
- Benchmark：加入 fio/IOR/mdtest/checkpoint workload runner，將結果寫回
  BenchmarkRecord，讓 profile selection/locality/cache policy 使用真實數據。
- Cache/checkpoint：實作 cache warmup/prefetch/eviction、cache residency
  proof、checkpoint async flush controller、flush checksum and retry/resume。

## G. Sonar Security 修復記錄

| Sonar 類型 | 原始狀態 | 本次處理 | 剩餘條件 |
|---|---|---|---|
| `secrets:S6698` workflow DB URL | `.github/workflows/backend-quality-gate.yml` had password-bearing `TEST_DATABASE_URL`。 | 改為 passwordless local CI URL，Postgres service uses `POSTGRES_HOST_AUTH_METHOD=trust` for ephemeral CI DB；本地 Sonar security issue total=0。 | 需下一次 CI/Cloud analysis 關閉 remote issue。 |
| `githubactions:S7637` action pinning | workflow used mutable tags (`@v4`, `@v5`, etc.)。 | 所有 `uses:` 改成 40-char SHA，comment 保留原 tag。 | 後續升級 action 需手動查 tag SHA。 |
| `kubernetes:S6431` coturn hostNetwork | SonarCloud automatic analysis still saw `hostNetwork: true`。 | 未改變 coturn manifest；TURN relay 需要 host networking，manifest 已有 `automountServiceAccountToken: false`、non-root UID/GID、read-only root filesystem、RuntimeDefault seccomp、drop capabilities、bounded relay ports、secret-backed auth。 | 需在 SonarCloud UI Analysis Scope 做 accepted-risk exclusion，或改 CI-based scanner 使用 `sonar-project.properties` exclusions；若未來能無 hostNetwork 運作，再移除。 |
| `docker:S6471` backend/per-service Dockerfiles | SonarCloud lists deleted per-service Dockerfiles plus backend root finding。 | Current repo only has `backend/Dockerfile` and Selkies Dockerfile; backend already `USER app:app`; deleted per-service issues should close on reanalysis。 | Remote Sonar refresh required。 |
| `docker:S6471` Selkies external image recipe | Selkies Dockerfile has no explicit `USER` and inherits upstream GPU desktop runtime。 | 未改變 external Selkies runtime recipe；本地 scanner excludes `backend/streaming/**` via `sonar-project.properties` because root-runtime compatibility is tracked separately。 | SonarCloud automatic analysis cannot use wildcard source exclusions in `.sonarcloud.properties`; use UI Analysis Scope or CI-based scanner config before claiming remote closure。 |
