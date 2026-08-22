# S0.1 Platform Core 进展

> Architecture authority：`topabomb/measix-architecture@dbb56952ab1cf60fa55e4cbb8d14ee70eda43a48`  
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
| C6 Browser + Hub + Relay Product/System E2E | 🔴 Incomplete | Browser Golden Path 仍不完整（缺少 Resource authoring/Policy/Pricing/Validate/Preview/Publish/Activation 的真实 UI 操作）；Resource Baseline 仍有假 Green（CPU 未实现、GoRoutines 读取 OS Threads 而非 goroutines、first-byte 非 Relay overhead）；Hub crash A-E 语义未完全匹配；Relay concurrency RLY-CON-005/006 断言不足 |
| C7 Client Contract Freeze Gate | ⛔ Blocked | C7 evidence pipeline 存在逻辑自锁（clean replay 被自身 CAP-C7-002 阻断）；collect-artifacts exit code 可能被覆盖；clean replay 未真正重放 Browser/Test Client/4 profiles/Usage；须在 C6 Green + adapter VERIFIED + baseline GREEN 后执行 |

## 当前有效证据

- 默认 `ci-gate` 只证明当前 PR head 的 deterministic T0–T3 baseline；**不证明 C6/C7/S0.1 Freeze**。
- `make s01-candidate-test` 提供 deterministic candidate system scenarios；只有在 exact candidate SHA 上实际执行后的结果才是 C6 evidence。
- `console/e2e/golden-path.spec.ts` + `console/e2e/topology-security.spec.ts` + `scripts/e2e-harness.mjs` 已提供 real-browser/clean-environment 基础设施，包括 topology security 验证（`/internal/*` 不可达）和 session 持久性；Playwright JSON reporter 输出到 `.artifacts/e2e-playwright.json`。
- `scripts/collect-adapter-qualification.mjs` 已实现自动化 Hub→Relay→real Adapter qualification flow；须用真实 endpoint/key 执行。
- `scripts/collect-baseline.mjs` / `baseline_test.go` 已扩充为采集 architecture §17 指标（Hub/Relay 读取真实进程 RSS/threads）；GREEN 由 metric completeness 计算。
- `scripts/freeze-manifest.mjs` 从真实 artifact 编译结果而非硬编码；scenario definitions 外置到 `scripts/scenario-definitions.json`；artifact 缺失或 SHA 不匹配时 hard fail。
- `make freeze-gate` 为 authoritative C7 entry point。Real adapter qualification 须单独先执行。

## 本轮修正中的 P0 问题

以下问题已识别并正在修正：

1. ✅ **CPU 度量假 Green 已修复** — `process_metrics.go` 中 Linux/Windows 的 `CPUPercent` 之前存储的是累计 CPU 时间（jiffies/秒），而非真实百分比。已改为 interval delta 方式：`ΔprocessCPU / Δwall / cores * 100`。第一次调用 prime snapshot，第二次调用计算 delta。`baseline_test.go` 也增加了 prime+wait 逻辑。
2. ✅ **Resource Baseline sanity check 已修复** — `collect-baseline.mjs` 之前只检查 metric 存在/类型正确就判 GREEN。已增加数值合理性验证（min/max range、NaN/Infinity 检查），RSS=0 或 CPU>100*numCores 等无意义值不再 GREEN。
3. ✅ **Generation N→N+1 测试已修复** — `TestCAPC6004PublishNewGeneration` 之前在得到 428 后重新创建 Enrollment 获取 N+1，而不是让同一个已绑定 Client Session 通过 Managed State→Snapshot 完成更新。已改为固定同一个 access/refresh session：N 成功 → Publish N+1 → N 请求 428 + forwarded=false → `GET managed/state` → 下载 Snapshot N+1 + ETag validation → 新 `interactionId` 使用 N+1 成功。不重新 Enrollment。
4. ✅ **Clean replay 未运行 Browser Golden Path 已修复** — `replay-freeze.mjs` 之前只启动 fresh Hub/Relay 环境后调用 Go candidate tests，这些 tests 各自启动自己的 HubEnv。已增加在同一 fresh 环境中运行 production Playwright Browser Golden Path（SPA proxy + deterministic adapter + Playwright）。
5. ✅ **Adapter Qualification identity 不符合 contract 已修复** — `collect-adapter-qualification.mjs` 之前 `adapterVersion` 是 `configRevision + upstreamId` 的 hash，不是实际 Adapter/version。已改为通过 `probeAdapterIdentity` 从真实 endpoint `/v1/models` 和 server headers 探测实际 adapterName/version/build，并记录探测方式。
6. 🔧 **Browser Golden Path 假闭合** — `golden-path.spec.ts` 实际只做 login、create user、navigation、logout；缺少 Resource authoring、Policy/Pricing、Validate、Preview、Publish、Activation 的完整 UI 操作。修正为单一 `test()` + `test.step()` 覆盖完整连续 workflow。
7. 🔧 **Hub topology 已修复** — Hub 已有独立 private/internal listener；public router 不注册 `/internal/*`；SPA proxy 也阻挡 `/internal/*`。T3 直接验证 public listener 上 `/internal/*` 不存在。
8. 🔧 **C7 逻辑自锁** — 改为两阶段：candidate manifest → clean replay → replay artifact PASS → final freeze manifest。

## 下一轮阻塞问题与顺序

1. ⏳ **完成 Browser C6 Golden Path**  
   通过 production SPA + real Hub/Relay，只使用 Admin UI 完成 User/Enrollment、Secret、Upstream Test/Apply、Model/TTS/ASR/MCP、Policy/Pricing、Validate、Review/Preview、Publish、Activation、Usage/System；不得用 Admin API/DB/internal shortcut 替代被测 UI 操作。

2. ✅ **收紧 T4.1 public topology**  
   public platform origin 不暴露 `/internal/*`；Hub↔Relay internal control 保持私有边界，并增加明确的不可达验证。

3. ⏳ **修正 generation update system proof**  
   generation N 请求成功 → Publish N+1 → stale request 428/no-forward → **同一 Client Session** 获取 managed state / Snapshot N+1 → 新 interaction 成功；不得通过重新 Enrollment 绕过 update flow，也不得自动 replay 原请求。

4. ⏳ **闭合 C7 evidence pipeline**  
   将 candidate Go results、production Browser Playwright JSON、static/contract、resource baseline、real Adapter qualification 全部作为 exact-SHA machine-readable evidence；artifact 缺失、SHA 不匹配或 NOT_EXECUTED 必须 hard fail。

5. ⏳ **完成 Resource Baseline**  
   补齐 architecture 要求的 Hub/Relay RSS/CPU、Relay first-byte overhead、concurrent stream memory、TTS buffering、ASR memory/temp-disk、cancel cleanup、usage backlog drain、SQLite growth；缺 required metric 不得标记 GREEN。

6. ⏳ **完成 Real Adapter Qualification runner**  
   实现完整自动化 Hub→Relay→real Adapter qualification flow（Secret/Upstream/Test/Apply/Resources/Publish/runtime 请求/usage 验证）；须用真实 endpoint/key 执行。

7. ⏳ **执行 Real Adapter Qualification**  
   用真实 endpoint/key 运行 `make collect-adapter-qualification ENDPOINT=... KEY=...` 生成 VERIFIED artifact。

8. ⏳ **执行 C7 Freeze + Clean Replay**  
   固定 exact architecture/core/build/contract identities，所有 required evidence Green 后生成 manifest；再从该 manifest 在 clean environment 重放 required S0.1 system path，`CAP-C7-002` Green 后才允许声明 S0.1 Freeze。

9. ⏳ **C7 之后才开始 S0.2 Android**。

## Freeze 状态

当前**不存在有效 S0.1 Freeze**。`docs/s0-freeze-manifest.json` 只能作为成功 C7 candidate 的 generated evidence；不得保留 placeholder/stale manifest。

最终 manifest 必须固定 architecture/core/build/contract/adapter/scenario identities，并引用可持久复现的 qualification/system evidence。任何 candidate SHA 变化都必须重新建立受影响的 candidate evidence。

## 状态维护规则

本文件只维护**当前实现状态、可用证据和剩余阻塞项**，不复制 architecture requirement 细节，也不保存历史审查报告或旧 CI 运行日志。

Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。Checkpoint completion 必须能够指向当前 architecture baseline 与 exact implementation candidate 对应的 executable evidence。
