# S0.1 Platform Core 进展

> Architecture authority：`topabomb/measix-architecture@cc60f8f`  
> Implementation branch：`agent/s0-platform-core`  
> 当前阶段：**S0.1 Managed Capability Delivery**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

S0 Core 基础已经建立：Identity/Enrollment、Draft/Release/Snapshot、RuntimeControlState、Relay admission/transport、Usage spool/ledger/pricing、Admin Console 与 deterministic system-test foundations 均已有实现和回归测试。

**S0.1 尚未完成。** Client Control/Snapshot v1 仍是 pre-freeze；不得进入 S0.2 Android，也不得宣称 S0 Exit。

2026-08-20 architecture 更新后，旧 C1/C2/C3 completion evidence 已失效；当前实现已按新的 visual authoring / review / validation navigation 要求补齐相应功能和 component evidence。历史 Green 仅作为回归参考，不能替代当前 candidate gate。

## C0–C7 当前状态

| Checkpoint | 状态 | 当前结论 |
|---|---|---|
| C0 Contract Audit & Freeze Preparation | ✅ Green | required profile OpenAPI/fixtures/generated types/backend validation 已闭合；Client contract 仍为 pre-freeze |
| C1 Upstream Operational | ✅ Green | candidate edit、Secret replace、Test/Apply、candidate/active revision、409/recovery 已实现 |
| C2 Managed Resource Editor | ✅ Green | Provider/Model/TTS/ASR/MCP/Policy typed authoring、binding、validation navigation、relationship view 已实现 |
| C3 Snapshot Projection & Preview | ✅ Green | canonical projection、Review、Client Snapshot Preview、Publish progress 已实现 |
| C4 Runtime Reference Profile | ✅ Green | deterministic T2/T3 已证明 Chat/SSE、TTS binary、ASR multipart、MCP 与关键 Relay admission/transport；不等于 real Adapter qualification |
| C5 Usage / Pricing / Observability | ✅ Green（component） | filters、request detail、pricing、summary、Overview/System 已有 component/backend evidence；仍需 C6 产品闭环证明 |
| C6 Browser + Hub + Relay Product/System E2E | 🔶 In Progress | deterministic candidate scenarios 与 resource baseline 已通过；Browser Golden Path 的已知问题（Apply confirm 框、模糊 `/active/i` 断言、Browser/Test Client 流量顺序、Adapter 生命周期、环境清理）已在代码中修复，但尚未在当前 candidate SHA 上执行验证。Browser 测试已拆分为 authoring/publish + usage/system 两阶段，由 `candidate-orchestrator.mjs` 统一编排。详细见 `docs/s01-alignment-audit-plan.md` |
| C7 Client Contract Freeze Gate | ⛔ Blocked | 当前不能进入 Freeze：C6 Golden Path 尚未在当前 SHA 执行验证、real Adapter qualification 尚未执行、既有 artifact 非当前 SHA。待 C6 Green、四个 required profile VERIFIED、静态/迁移/生成物证据由当前候选重新生成后再执行 |

## 当前有效证据

- 默认 `ci-gate` 只证明当前 PR head 的 deterministic T0–T3 baseline；**不证明 C6/C7/S0.1 Freeze**。
- `make s01-candidate-test` 提供 deterministic candidate system scenarios；只有在 exact candidate SHA 上实际执行后的结果才是 C6 evidence。
- `console/e2e/golden-path-authoring.spec.ts` + `console/e2e/golden-path-usage.spec.ts` + `console/e2e/topology-security.spec.ts` + `scripts/e2e-harness.mjs` + `scripts/candidate-orchestrator.mjs` 已提供 real-browser/clean-environment 基础设施，包括拆分的 authoring/publish + usage/system 两阶段、topology security 验证（`/internal/*` 不可达）和 session 持久性；Playwright JSON reporter 输出到 `.artifacts/e2e-playwright.json`。
- `scripts/collect-adapter-qualification.mjs` 已实现自动化 Hub→Relay→real Adapter qualification flow；须用真实 endpoint/key 执行。
- `scripts/collect-baseline.mjs` / `baseline_test.go` 已扩充为采集 architecture §17 指标（Hub/Relay 读取真实进程 RSS/threads）；GREEN 由 metric completeness 计算。
- `scripts/freeze-manifest.mjs` 从真实 artifact 编译结果而非硬编码；scenario definitions 外置到 `scripts/scenario-definitions.json`；artifact 缺失或 SHA 不匹配时 hard fail。
- `make freeze-gate` 为 authoritative C7 entry point。Real adapter qualification 须单独先执行。

## 审计对齐修复（2026-08-27）

以下问题已识别并已在代码中修复，但尚未在当前 candidate SHA 上执行验证：

1. ✅ **CPU 度量假 Green 已修复** — `process_metrics.go` 中 Linux/Windows 的 `CPUPercent` 之前存储的是累计 CPU 时间（jiffies/秒），而非真实百分比。已改为 interval delta 方式：`ΔprocessCPU / Δwall / cores * 100`。第一次调用 prime snapshot，第二次调用计算 delta。`baseline_test.go` 也增加了 prime+wait 逻辑。
2. ✅ **Resource Baseline sanity check 已收紧** — `collect-baseline.mjs` 之前只检查 metric 存在/类型正确就判 GREEN。已增加数值合理性验证（min/max range、NaN/Infinity 检查），RSS=0 或 CPU>50% 等无意义值不再 GREEN。idle RSS 上限收紧至 500MB，idle CPU 上限收紧至 50%。
3. ✅ **Generation N→N+1 测试已修复** — `TestCAPC6004PublishNewGeneration` 之前在得到 428 后重新创建 Enrollment 获取 N+1，而不是让同一个已绑定 Client Session 通过 Managed State→Snapshot 完成更新。已改为固定同一个 access/refresh session：N 成功 → Publish N+1 → N 请求 428 + forwarded=false → `GET managed/state` → 下载 Snapshot N+1 + ETag validation → 新 `interactionId` 使用 N+1 成功。不重新 Enrollment。
4. ✅ **Clean replay 未运行 Browser Golden Path 已修复** — `replay-freeze.mjs` 之前只启动 fresh Hub/Relay 环境后调用 Go candidate tests，这些 tests 各自启动自己的 HubEnv。已增加在同一 fresh 环境中运行 production Playwright Browser Golden Path（SPA proxy + deterministic adapter + Playwright）。
5. ✅ **Adapter Qualification identity 不符合 contract 已修复** — `collect-adapter-qualification.mjs` 之前 `adapterVersion` 是 `configRevision + upstreamId` 的 hash，不是实际 Adapter/version。已改为通过 `probeAdapterIdentity` 从真实 endpoint `/v1/models` 和 server headers 探测实际 adapterName/version/build，并记录探测方式。
6. ✅ **Browser Golden Path 假闭合已修复** — `golden-path.spec.ts` 之前 Policy 只检查 render、不实际操作；缺少 Pricing 创建。已改为真正切换 Policy allowLocal toggles（toggle ON→OFF），并增加 Phase 4g：导航到 Usage→Pricing tab，点击 Add Rule，填写 unit price，Save。同时给 `PricingPanel.vue` 添加 `data-cy` 属性（`pricing-add-rule-btn`、`pricing-save-btn`、`pricing-unit-price`）以支持测试定位。
7. ✅ **Hub topology 已修复** — Hub 已有独立 private/internal listener；public router 不注册 `/internal/*`；SPA proxy 也阻挡 `/internal/*`。T3 直接验证 public listener 上 `/internal/*` 不存在。
8. ✅ **C7 逻辑自锁已修复** — 改为两阶段：candidate manifest → clean replay → replay artifact PASS → final freeze manifest。`replay-freeze.mjs` 已集成 Playwright Browser Golden Path 执行。
9. ✅ **Upstream Location header 剥离已修复** — `sanitizeResponseHeaders` 之前只剥离 `Set-Cookie` 和 hop-by-hop headers，未剥离 `Location`。REGRESS-SEC-020 测试发现 Relay 将上游 200 OK 响应中的 `Location` header 转发给客户端。已增加 `location` 到剥离列表。
10. ✅ **Windows process metrics 已修复** — `process_metrics.go` 的 `readWindowsProcMetrics` 使用已弃用的 `wmic` 命令，在 Windows 11 上不可用导致 RSS=0。已改为使用 PowerShell `Get-CimInstance Win32_Process` 采集 WorkingSetSize/ThreadCount/KernelModeTime/UserModeTime。
11. ✅ **Publish invalid_draft 已修复** — `openReview()` 之前不调用 `draft.validate()` 刷新验证结果，导致 `publish()` 使用 stale `validationResult` 构建 `acknowledgedWarningCodes`。后端在 publish 时重新 validate 产生 `upstream_not_active` warning，但前端未将其包含在 acknowledged codes 中，返回 `invalid_draft`。已改为 `openReview` 先 `await draft.validate()` 确保 warning codes 最新。同时新增 `pollUntilSettled` 循环轮询，避免异步 activation 未完成时 UI 状态卡在 APPLYING。

## 下一轮阻塞问题与顺序

1. ⏳ **在当前 candidate SHA 上执行 Browser C6 Golden Path**  
   已修复 Apply confirm 接受、精确终态断言、拆分 Browser 阶段、统一 Adapter 生命周期和环境清理。需在当前 candidate SHA 上执行 `node scripts/candidate-orchestrator.mjs`（或 `node scripts/e2e-harness.mjs`）验证 Golden Path 真实通过。

2. ✅ **收紧 T4.1 public topology**  
   public platform origin 不暴露 `/internal/*`；Hub↔Relay internal control 保持私有边界，并增加明确的不可达验证。

3. ✅ **修正 generation update system proof**  
   `TestCAPC6004PublishNewGeneration` 和 `TestCAPC6004EnhancedNoForwardAndUsageGeneration` 已改为：generation N 请求成功 → Publish N+1 → stale request 428/no-forward → **同一 Client Session** 获取 managed state / Snapshot N+1 → 新 interaction 成功；不重新 Enrollment。

4. ✅ **闭合 C7 evidence pipeline**  
   candidate Go results、production Browser Playwright JSON、static/contract、resource baseline、real Adapter qualification 全部作为 exact-SHA machine-readable evidence；artifact 缺失、SHA 不匹配或 NOT_EXECUTED 必须 hard fail。

5. ✅ **完成 Resource Baseline（deterministic GREEN）**  
   Hub/Relay RSS/CPU（interval delta）、concurrent stream memory、cancel release time、SQLite growth 已实现；CPU 假 Green 已修复（`process_metrics.go` interval delta）；`collect-baseline.mjs` 已增加 sanity check（min/max range、NaN/Infinity）。Windows `wmic`→PowerShell CIM 修复后 baseline GREEN：Hub idle RSS 17MB/CPU 0.04%，Relay idle RSS 13MB/CPU 0%。

6. ✅ **完成 Real Adapter Qualification runner**  
   `collect-adapter-qualification.mjs` 已实现完整自动化 Hub→Relay→real Adapter qualification flow；`probeAdapterIdentity` 从真实 endpoint `/v1/models` 和 headers 探测实际 adapterName/version/build。须用真实 endpoint/key 执行。

7. ✅ **完成 Relay 并发测试**  
   `concurrency_test.go` 新增 `TestRLYCON005CancelStorm`（20 并发 stream + cancel，验证 RSS 回落和 Relay 可响应）和 `TestRLYCON006ControlApplyNoUsageBlock`（并发 usage 请求 + upstream apply，验证不互锁）。覆盖 architecture §9 RLY-CON-005/006。已修复 RLY-CON-006 double-close channel（拆分为 stopCh/doneCh）和 RLY-CON-005 假证明问题（移除 ClearCancelled 后重测，改为 storm 后直接验证 adapter cancel，RSS 阈值从 50MB 收紧至 20MB）。

8. ✅ **Pricing meter 枚举对齐**  
   OpenAPI `PricingRule.meter` 和 `UsageSummary.semanticMeters.meter` 从自由字符串改为 `$ref: PricingMeter` enum（INPUT_TOKENS/OUTPUT_TOKENS/CACHED_TOKENS/CHARACTERS/AUDIO_SECONDS/REQUESTS）。Go 后端 `validMeter` 函数验证所有 meter 输入。前端 `PricingPanel.vue` meter 选项同步更新。

9. ✅ **Generation N→N+1 安全断言收紧**  
   `TestCAPC6004EnhancedNoForwardAndUsageGeneration` 增加 `managed_snapshot_required` code 断言和 no-replay 二次验证。

10. ⏳ **执行 Real Adapter Qualification**  
   用真实 endpoint/key 运行 `make collect-adapter-qualification ENDPOINT=... KEY=...` 生成 VERIFIED artifact。

11. ⏳ **执行 C7 Freeze + Clean Replay**  
   固定 exact architecture/core/build/contract identities，所有 required evidence Green 后生成 manifest；再从该 manifest 在 clean environment 重放 required S0.1 system path，`CAP-C7-002` Green 后才允许声明 S0.1 Freeze。

12. ⏳ **C7 之后才开始 S0.2 Android**。

## Freeze 状态

当前**不存在有效 S0.1 Freeze**。`docs/s0-freeze-manifest.json` 只能作为成功 C7 candidate 的 generated evidence；不得保留 placeholder/stale manifest。

最终 manifest 必须固定 architecture/core/build/contract/adapter/scenario identities，并引用可持久复现的 qualification/system evidence。任何 candidate SHA 变化都必须重新建立受影响的 candidate evidence。

## 状态维护规则

本文件只维护**当前实现状态、可用证据和剩余阻塞项**，不复制 architecture requirement 细节，也不保存历史审查报告或旧 CI 运行日志。

Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。Checkpoint completion 必须能够指向当前 architecture baseline 与 exact implementation candidate 对应的 executable evidence。
