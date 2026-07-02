# Archive/Image Build and HPC Storage Audit

Date: 2026-06-30

_Re-verified 2026-07-01: archive parsing/validation (path traversal,
symlink/hardlink, zip-bomb, file-count/depth/length limits, deterministic
digest) and source-aware idempotency fingerprinting are now implemented and
merged (`imageregistry/buildcontext.go`, `imageregistry/handler.go`);
corrections are marked inline below with "(2026-07-01 更新/已解決)". HPC
storage findings (§D) and build execution/dispatch (BuildKit/Tekton/Harbor
push/SBOM/scan/sign, §B/§E/§F) are unchanged by this pass and remain
accurate._

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
3. （2026-07-01 更新）現在有 tar.gz/zip archive parser（含 path traversal、
   symlink/hardlink、zip bomb 防護與 deterministic digest，見
   `imageregistry/buildcontext.go`），但傳輸方式是 JSON body 內的 base64
   （非 multipart form upload），且沒有 object-storage build-context upload、
   BuildKit/Tekton job dispatch、Harbor push、SBOM/scan/sign/attestation
   實際狀態轉移。
4. （2026-07-01 更新）Dockerfile/context/storage_path/build_args 在 API
   fixture 中是 optional contract 欄位；`createBuild` 現在會 hash Dockerfile
   內容、驗證並 hash context archive、保存 `source_digest`（見
   `imageregistry/handler.go`），但仍未派工使用這些 source 執行實際 build。
5. （2026-07-01 更新）image build idempotency fingerprint 現在包含 Dockerfile
   SHA-256、context archive digest、context reference、storage path、
   build args（見 `imageBuildIdempotencyFingerprint`），同一 `Idempotency-Key`
   換不同 source 會回傳 `409 Conflict`。仍未包含 from-storage 參照物件本身的
   content checksum（只有 path 字串）或獨立 source revision 欄位。
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
| tar.gz upload | 有。`image-build.md` 把 `tar.gz / zip upload` 列為 supported source。 | （2026-07-01 更新）有基本 archive parser/validator（`imageregistry/buildcontext.go`：格式偵測、path traversal/symlink/hardlink/zip bomb 防護、file-count/depth/length 限制、deterministic digest），但透過 JSON body 內 base64 傳輸（interim `context_archive` 欄位），非 multipart form upload。 | `imageregistry/buildcontext.go`; `handler.go` `imageBuildContextArchiveDigest`。 | archive 解析/驗證已實作；multipart upload 仍未實作。 |
| zip upload | 有。與 tar.gz 同列。 | （2026-07-01 更新）同上，zip 格式已由同一 validator 支援（含 zip bomb 防護），但同樣非 multipart upload。 | 同上。 | archive 解析/驗證已實作；multipart upload 仍未實作。 |
| Dockerfile + build context | 有。CLI local Dockerfile + context 是 supported source，fixture 也有 `dockerfile`/`context`。 | （2026-07-01 更新）handler 現在會讀取並 hash Dockerfile 內容、驗證並 hash context archive，保存為 `source_digest`；但不做 Dockerfile 語法驗證，也不會實際使用這些 source 執行 build。 | `docs/acceptance/image-build.md:17`; fixture `image-registry-dockerfile-build.json:22-47`; `imageregistry/handler.go` `imageBuildSourceFingerprint`/`imageBuildContextArchiveDigest`。 | source 內容已 hash/驗證/保存；build 執行仍未實作。 |
| from-storage build | 有。user/group/project storage 是 supported source，`/from-storage` route 存在。 | 只有 API contract/metadata。沒有 storage path permission、mount-plan lookup 或 object/PVC dispatch。 | Route at `handler.go:89`; `startStorageImageBuild` only calls `createBuild` at `handler.go:521-523`; fixture optional `storage_path` at `image-registry-storage-build.json:22-40`。 | 只有 metadata queue。 |
| archive build context | 有。Build flow 寫「upload context to object storage」。 | （2026-07-01 更新）archive 結構驗證與 SHA-256 content digest 已實作並保存為 `source_digest`，但沒有 object-storage upload 或實際 unpack/extract 供 build 使用。 | `docs/acceptance/image-build.md:31-33`; `imageregistry/buildcontext.go`; `handler.go` 保存 `source_digest`。 | digest/驗證已實作；object-storage upload 與 unpack 仍未實作。 |
| image build pipeline | 有。Build flow 描述 quota, upload/mount, Tekton, BuildKit, Harbor, SBOM, scan, sign, allow-list。 | 沒有 pipeline executor。只建立 `queued` row 和 `ImageBuildStarted` event。 | `docs/acceptance/image-build.md:24-41`; `handler.go:691-697` emits event and returns 202。 | 只有 metadata queue，不是 pipeline。 |
| BuildKit | 有。Acceptance IMG-010 指 rootless BuildKit through Tekton。 | 沒有 BuildKit Job/PipelineRun dispatch。 | `docs/acceptance/image-build.md:67-68`; code search shows no product BuildKit executor in imageregistry。 | 文件/AC 超前實作。 |
| Tekton | 有。Build flow 寫 Tekton PipelineRun。 | 沒有 PipelineRun manifest/controller。 | `docs/acceptance/image-build.md:31-34`; no Tekton dispatch in `imageregistry` handler/service。 | 文件/AC 超前實作。 |
| Harbor push | 有。Build flow/IMG-014 要 push Harbor。 | 沒有 build push；只有 Harbor health/catalog sync/read metadata。 | `docs/acceptance/image-build.md:35,72`; `handler.go:697` only attaches Harbor degraded metadata; catalog sync is separate Harbor artifact read path。 | live Harbor foundation/sync exists，但 build push 沒實作。 |
| SBOM | 有。Build flow 和 IMG-016 要 SBOM。 | 沒有生成。queued record 只設 `sbom_status="pending"`。 | `docs/acceptance/image-build.md:36,74`; `handler.go:676-681`。 | 只有 metadata/status placeholder。 |
| image scan | 有。Build flow 和 IMG-017 要 scan result。 | 沒有 build scan lifecycle。queued record 只設 `scan_status="pending"`；Harbor-side scan evidence 是獨立 synthetic image。 | `docs/acceptance/image-build.md:37,75`; `docs/acceptance/gap-analysis.md:246-274`; `handler.go:680`。 | 只有 metadata；不是 build pipeline scan。 |
| signing / attestation | 有。Build flow 和 IMG-018 要 signature/attestation。 | 沒有 Cosign/sign/attestation executor。queued record 只設 signature pending。 | `docs/acceptance/image-build.md:38,76`; `handler.go:678-681`。 | 只有 metadata/status placeholder。 |
| allow-list enforcement | 有。IMG-019/020/021 要 digest allow-list。 | 部分實作，但不屬於 build completion pipeline。catalog publish guard 和 scheduler submit allow-list guard 是 local/static/in-code evidence。 | `docs/acceptance/gap-analysis.md:546-574`; `handler.go:96-98` owner-read allow-list contract; scheduler admission tests exist。 | 部分 admission guard；build 成功後自動 allow-list workflow 未完成。 |
| multipart upload | 文件對 tar.gz/zip imply upload。 | image build 沒有。multipart 只出現在 media upload tests，不在 imageregistry build。 | `handler.go:623` JSON decode；no `ParseMultipartForm`/`FormFile` in image build path。 | 沒實作。 |
| Dockerfile content handling | fixture optional `dockerfile`。 | （2026-07-01 更新）handler 讀取並 SHA-256 hash Dockerfile 內容、納入 idempotency fingerprint；不做 Dockerfile 語法驗證，不保存原始內容。 | `image-registry-dockerfile-build.json:22-47`; `imageregistry/handler.go` `imageBuildSourceFingerprint`。 | 內容已 hash/納入 fingerprint；語法驗證與內容持久化仍未實作。 |
| build context handling | fixture optional `context`。 | （2026-07-01 更新）`context` 參照本身只作為 fingerprint 欄位；若透過 `context_archive`（base64 JSON）提供實際 archive，則會被解析、驗證、hash。 | `image-registry-context-build.json:22-45`; `imageregistry/buildcontext.go`; `imageregistry/handler.go`。 | archive 內容路徑已實作；不上傳 object storage。 |
| object storage upload | Build flow 宣稱 upload context to object storage。 | image build 沒有 object-store call。 | `docs/acceptance/image-build.md:31-33`; image build code only store record。 | 沒實作。 |
| source digest / checksum | IMG-025 事件要 source type/resources/image digest/scan/allow-list decision；from-storage fixture有 source path。 | （2026-07-01 更新）build idempotency fingerprint 現在含 source checksum（Dockerfile SHA-256、context archive digest），並保存為 `source_digest`；`image_digest`（最終 build 出的 image digest，非 source digest）初始仍是 `""`，因為尚無 build 執行器產出真正的 image。 | `imageregistry/handler.go` `imageBuildIdempotencyFingerprint`、`source_digest`。 | source content digest 已實作；最終 image digest 待 build 執行器落地。 |
| unpack / extract | archive build implies extraction。 | 沒有。 | no tar/zip extractor in product image build path。 | 沒實作。 |
| path traversal 防護 | archive extraction 必須具備。 | （2026-07-01 更新）已實作：拒絕絕對路徑與 `../` escape（`normalizeBuildContextPath`）。 | `imageregistry/buildcontext.go`。 | 已完成（結構驗證層；尚無 extraction pipeline 可套用）。 |
| symlink / hardlink 防護 | archive extraction 必須具備。 | （2026-07-01 更新）已實作：tar symlink/hardlink 與 zip symlink 一律拒絕。 | `imageregistry/buildcontext.go`。 | 已完成。 |
| zip bomb 防護 | zip upload 必須具備。 | （2026-07-01 更新）已實作：compressed 與 running uncompressed total 皆有上限（streaming 檢查，非信任 header）。 | `imageregistry/buildcontext.go`。 | 已完成。 |
| max archive size / max file count | archive upload 必須具備。 | （2026-07-01 更新）已實作：max archive bytes、max file count、max path depth、max path length 皆有限制。 | `imageregistry/buildcontext.go`。 | 已完成。 |
| build logs | 有 IMG-011。 | 部分。GET logs returns stored string with redaction; no live executor streaming/tailing。 | `handler.go:543-558`; redaction at `handler.go:561-583`; gap says no live logs at `gap-tracker.md:641-646`。 | local/static log response only。 |
| cancel build | 有 IMG-012。 | 部分。DELETE sets metadata status `cancelled`; no executor/K8s termination。 | `handler.go:586-615`; gap says no live executor cancellation at `gap-tracker.md:591-600`。 | metadata cancellation only。 |
| build status update/result digest | 有 IMG-015/017/018/019/025。 | queued create only; no completion controller writing digest or final scan/sign/SBOM states。 | `handler.go:675-681`。 | 只有 pending metadata。 |

## C. 文件與程式碼偏離清單

| 偏離項目 | 文件說法 | 程式碼實際狀況 | 風險 | 建議 |
|---|---|---|---|---|
| Supported Build Sources 用詞暗示已支援 | `image-build.md` 列出 local Dockerfile+context、tar.gz/zip、user/group/project storage。 | （2026-07-01 更新）三個 REST route 存在；handler 現在會 hash/驗證 Dockerfile/context archive 內容（見 §A.4），但仍不會實際執行 build，只建立 queued metadata。 | 使用者會以為可上傳 archive 或從 storage build 並得到真正的 image，實際只得到已驗證的 queued metadata。 | 把「Supported」改成「Target/Planned」或在同段加 Current implementation status。 |
| Build Flow 描述完整 pipeline | 文件列 Tekton/BuildKit/Harbor/SBOM/scan/sign/allow-list。 | 沒有 executor；`createBuild` 只建 record/event。 | production readiness 誤導；queued build 永遠不會產出 image digest。 | 在文件中標明 flow 是 GA target；新增 live executor 前不要宣稱完成。 |
| Dockerfile fixture 讓人以為 handler 使用 Dockerfile | fixture request example 有 `dockerfile`、`context`、`build_args`。 | （2026-07-01 更新）handler 現在會 hash 這些欄位並納入 idempotency fingerprint；不同 Dockerfile/context/build_args 換同一 key 會回傳 `409 Conflict`，不會回放舊 build。 | 已解決：retry/provenance 錯誤已修正（見 `imageBuildIdempotencyFingerprint`）；build 執行本身仍未實作。 | 已完成 fingerprint 修正；下一步是實作 build 執行器。 |
| from-storage fixture 讓人以為有 storage permission | fixture optional `storage_path`。 | handler 不驗證 storage path、Project storage permission 或 mount-plan。 | 越權/錯誤語意風險，一旦 executor 補上容易漏做 trust-boundary。 | from-storage 必須接 storage service mount-plan/permission owner-read。 |
| idempotency fingerprint 缺 build source | 文件/fixtures允許 dockerfile/context/storage_path/build_args。 | （2026-07-01 更新）已解決：fingerprint 現在含 Dockerfile SHA-256、context archive digest、context reference、storage path、build args。 | 已解決；仍缺 from-storage 參照物件的 content checksum（只有 path 字串）與獨立 source revision 欄位。 | 補 from-storage content checksum 與 source revision 欄位。 |
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
- （2026-07-01 已解決）Image build idempotency fingerprint 現在含 source
  content（Dockerfile SHA-256、context archive digest、context reference、
  storage path、build args）。同一 `Idempotency-Key` 換不同 source 會回傳
  `409 Conflict`，不再回放舊 request。

### P1

- （2026-07-01 已解決）Archive 結構安全控制已實作（path traversal、
  symlink/hardlink/device/fifo/socket 拒絕、zip bomb 防護、max
  size/file-count/depth/length 限制、deterministic digest，見
  `imageregistry/buildcontext.go`）。仍缺：streamed multipart upload（目前是
  base64-in-JSON）、object-storage staging、實際 unpack/extract 供 build 使用。
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
- （2026-07-01 部分已完成）Archive upload pipeline：canonical 結構驗證、
  compressed/uncompressed bytes、entry count/path/depth/length、
  symlink/hardlink policy、SHA-256 digest 已實作
  （`imageregistry/buildcontext.go`）。仍待：新增 multipart endpoint（目前是
  base64-in-JSON）、streaming tar/zip reader 做實際 extraction 供 build 使用。
- （2026-07-01 部分已完成）Source provenance：build record 已保存 Dockerfile
  digest、context archive digest、`source_digest`；event payload（`rec.Data`）
  已透傳這些欄位。仍待：storage object ID/path/version 的 content checksum
  （目前只有 path 字串）、獨立 source revision 欄位。
- （2026-07-01 已完成）Idempotency fingerprint：已加入 Dockerfile content
  hash、context archive hash、storage path、build args、image reference、
  project ID、requested user、CPU/memory/timeout（見
  `imageBuildIdempotencyFingerprint`）。仍待：from-storage object 的 content
  checksum、source revision 欄位。
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
