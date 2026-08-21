# S0.1 Platform Core 进展

> Architecture authority：`topabomb/measix-architecture@6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b`  
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
| C6 Browser + Hub + Relay Product/System E2E | 🔴 Not Green | candidate/system 测试代码已建立，但完整 production Browser T4.1 Golden Path、真实产品 recovery/security 组合证据仍未完成并在 exact candidate 上执行 |
| C7 Client Contract Freeze Gate | 🔴 Blocked | C6 未 Green；real Adapter qualification、完整 resource baseline、Browser evidence wiring、clean replay 尚未闭合；没有有效 Freeze manifest |

## 当前有效证据

- 默认 `ci-gate` 只证明当前 PR head 的 deterministic T0–T3 baseline；**不证明 C6/C7/S0.1 Freeze**。
- `make s01-candidate-test` 提供 deterministic candidate system scenarios；只有在 exact candidate SHA 上实际执行后的结果才是 C6 evidence。
- `console/e2e/golden-path.spec.ts` 与 `scripts/e2e-harness.mjs` 已提供 real-browser/clean-environment 基础设施，但当前 Browser suite 仍是部分产品路径，不足以证明 architecture `CAP-C6-001` 全流程。
- `scripts/collect-adapter-qualification.mjs` 当前仍是 qualification entry point/placeholder；真实 endpoint qualification 尚未执行。
- `scripts/collect-baseline.mjs` / `baseline_test.go` 当前只覆盖部分延迟数据，尚未覆盖 architecture 要求的完整 RSS/CPU/streaming/multipart/cancel/spool/SQLite resource baseline。
- `scripts/freeze-manifest.mjs` 已从真实 artifact 编译结果而非硬编码 PASS，但当前还没有消费 Playwright Browser T4.1 evidence。
- 当前 `make freeze-gate` 也尚未自动执行 Browser T4.1、resource baseline、real Adapter qualification 和 `CAP-C7-002` clean replay，因此**不能作为完整 C7 Gate**。

## 下一轮阻塞问题与顺序

1. **完成 Browser C6 Golden Path**  
   通过 production SPA + real Hub/Relay，只使用 Admin UI 完成 User/Enrollment、Secret、Upstream Test/Apply、Model/TTS/ASR/MCP、Policy/Pricing、Validate、Review/Preview、Publish、Activation、Usage/System；不得用 Admin API/DB/internal shortcut 替代被测 UI 操作。

2. **收紧 T4.1 public topology**  
   public platform origin 不暴露 `/internal/*`；Hub↔Relay internal control 保持私有边界，并增加明确的不可达验证。

3. **修正 generation update system proof**  
   generation N 请求成功 → Publish N+1 → stale request 428/no-forward → **同一 Client Session** 获取 managed state / Snapshot N+1 → 新 interaction 成功；不得通过重新 Enrollment 绕过 update flow，也不得自动 replay 原请求。

4. **闭合 C7 evidence pipeline**  
   将 candidate Go results、production Browser Playwright JSON、static/contract、resource baseline、real Adapter qualification 全部作为 exact-SHA machine-readable evidence；artifact 缺失、SHA 不匹配或 NOT_EXECUTED 必须 hard fail。

5. **完成 Resource Baseline**  
   补齐 architecture 要求的 Hub/Relay RSS/CPU、Relay first-byte overhead、concurrent stream memory、TTS buffering、ASR memory/temp-disk、cancel cleanup、usage backlog drain、SQLite growth；缺 required metric 不得标记 GREEN。

6. **完成 Real Adapter Qualification**  
   按 architecture qualification spec 对 release 声称 VERIFIED 的 required profiles 形成真实 endpoint/config/version evidence；deterministic Adapter 不可替代。

7. **执行 C7 Freeze + Clean Replay**  
   固定 exact architecture/core/build/contract identities，所有 required evidence Green 后生成 manifest；再从该 manifest 在 clean environment 重放 required S0.1 system path，`CAP-C7-002` Green 后才允许声明 S0.1 Freeze。

8. **C7 之后才开始 S0.2 Android**。

## Freeze 状态

当前**不存在有效 S0.1 Freeze**。`docs/s0-freeze-manifest.json` 只能作为成功 C7 candidate 的 generated evidence；不得保留 placeholder/stale manifest。

最终 manifest 必须固定 architecture/core/build/contract/adapter/scenario identities，并引用可持久复现的 qualification/system evidence。任何 candidate SHA 变化都必须重新建立受影响的 candidate evidence。

## 状态维护规则

本文件只维护**当前实现状态、可用证据和剩余阻塞项**，不复制 architecture requirement 细节，也不保存历史审查报告或旧 CI 运行日志。

Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。Checkpoint completion 必须能够指向当前 architecture baseline 与 exact implementation candidate 对应的 executable evidence。
