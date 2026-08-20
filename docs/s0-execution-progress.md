# S0.1 Platform Core 进展

> Architecture authority：`topabomb/measix-architecture@6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b`  
> Implementation branch：`agent/s0-platform-core`  
> 本轮重新审查前的实现 head：`846781f86473db44d4c0a87e2379a8298efbb956`  
> 当前阶段：**S0.1 Managed Capability Delivery**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

S0 Core 基础已经建立：Identity/Enrollment、Draft/Release/Snapshot、RuntimeControlState、Relay admission/transport、Usage spool/ledger/pricing、Admin shell 和 deterministic runtime test foundations 均已有实现与回归测试。

**S0.1 尚未完成。** Client Control/Snapshot v1 仍是 pre-freeze；不得进入 S0.2 Android，也不得宣称 S0 Exit。

2026-08-20 的 architecture 更新把 Admin visual authoring / review / validation navigation 明确成 executable requirement，因此此前基于旧 architecture baseline 得出的 C1/C2/C3 Green 结论已失效。旧 Green 仍可作为底层回归证据，但不是当前 checkpoint completion evidence。

## C0–C7 当前状态

| Checkpoint | 状态 | 当前结论 |
|---|---|---|
| C0 Contract Audit & Freeze Preparation | ✅ Green | required profile 的 OpenAPI/fixtures/generated types/backend validation 已基本闭合；继续作为后续回归基线 |
| C1 Upstream Operational | 🟡 Partial | Create/Test/Apply 与 typed UpstreamConfig 已存在；仍需完成 existing candidate edit、Secret replace、save/discard/reset/disable、refresh recovery 与 apply-failure candidate preservation 的完整产品闭环和最新测试 |
| C2 Managed Resource Editor | 🟡 Partial | Provider/Model/TTS/ASR/MCP/Policy 基础编辑与 binding 已存在；仍需完成 collection → selected editor/detail、Policy Default Model/TTS/ASR、dirty/clean state、field/resource validation navigation，以及完整 relationship state 展示 |
| C3 Snapshot Projection & Preview | 🟡 Partial | canonical preview backend 与基础 UI 已存在；仍需完成真正的 Review & Publish（Added/Changed/Removed、Policy/Runtime impact、warnings）以及可审阅的 client-facing Snapshot projection |
| C4 Runtime Reference Profile | ✅ Green | deterministic Adapter/Test Client 已证明 Chat request/response+SSE、TTS binary、ASR multipart、MCP 及关键 Relay admission/transport 行为；这是 deterministic T2/T3 证据，不等于 real Adapter qualification |
| C5 Usage / Pricing / Observability | ✅ Green（component checkpoint） | filters、request detail、pricing、summary、Overview/System observability 已具备并有 component/backend 回归；仍须在 C6 Golden Path 中证明跨组件产品闭环 |
| C6 Browser + Hub + Relay Product/System E2E | 🔴 Not Green | 当前只有 bounded T3/system smoke；缺 production Admin real-browser T4.1 Golden Path、完整 Hub→Relay→Adapter→Usage 组合证明与 required recovery/security product scenarios |
| C7 Client Contract Freeze Gate | 🔴 Not Started | C6 未 Green；real Adapter qualification 未完成；没有有效 Freeze manifest |

## 当前有效证据如何解释

- `ci-gate` / backend / console / current system smoke：证明最新执行 SHA 对应的 T0–T3 deterministic baseline；**不证明 C6/C7/S0.1 Freeze**。
- `docs/s0-review-report.md`：旧 architecture baseline 下的历史审查/回归映射，只作 historical evidence，不再作为当前 S0.1 completion source。
- real Adapter qualification：必须按 architecture qualification spec 单独执行并形成证据；deterministic Adapter 不能替代。
- Browser T4.1：必须对 exact candidate SHA 显式执行 production SPA + real Hub + real Relay + deterministic Adapter/Test Client；默认 GitHub Actions 不执行该 gate。

## Freeze 状态

当前**不存在有效 S0.1 Freeze**。仓库不保留 placeholder/stale `docs/s0-freeze-manifest.json`；只有在 C6 required scenarios、real Adapter qualification 和 C7 contract evidence 全部 Green 后，才由 candidate verification 生成 machine-readable manifest。

有效 manifest 必须按 architecture S0.1 System Testing Spec / `docs/release.md` 固定 exact architecture/core/build/contract/adapter/scenario identities。任何旧 SHA 生成的 preliminary JSON 都不得继续叫 Freeze manifest。

## 下一步顺序

保持 `measix-s0-capability-delivery-implementation-decision.md` 的 C0–C7 顺序，不重新设计 Relay 或扩大 S0.1 scope：

1. 以最新 Admin Product Requirements + Admin Testing Spec 为准补齐 C1；
2. 补齐 C2 visual authoring、Policy defaults、validation navigation 与 relationship state；
3. 补齐 C3 Review → Snapshot Preview → Publish 产品闭环；
4. 建立/完成 Playwright real-browser T4.1 slices，组合 C6 clean-environment Golden Path；
5. 完成至少一套 required real Adapter qualification；
6. exact candidate 全部 Green 后生成 C7 Freeze manifest；
7. C7 之后才开始 S0.2 Android。

## 状态维护规则

本文件只维护**当前实现状态和剩余缺口**，不复制 architecture requirement 细节。Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。每次宣称 checkpoint Green，必须能指向当前 architecture baseline 与当前 implementation SHA 对应的 executable evidence。
