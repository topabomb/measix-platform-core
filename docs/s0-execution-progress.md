# S0.1 Platform Core 当前进展

> Architecture baseline: `topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`  
> 当前阶段：**S0.1 Managed Capability Delivery**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

现有分支已经具备 S0 Core 的主要基础：Identity/Enrollment、Draft/Release/Snapshot 基础、Relay desired-state apply/admission/transport、Usage spool/ledger/pricing 以及 Admin shell。

但 **S0.1 尚未完成，Client Control/Snapshot v1 仍是 pre-freeze，当前不能进入 S0.2 Android，也不能宣称 S0 Exit。**

历史 I0–I5 仅作为已有实现与回归证据标签，不再作为当前交付顺序。

## C0–C7 状态

| Checkpoint | 状态 | 当前主要缺口 |
|---|---|---|
| C0 Contract Audit & Freeze Preparation | 未完成 | Client/Admin OpenAPI、Snapshot fields/enums、TTS voice、ASR managed semantics、MCP auth ownership、usage/preview contract 与 fixtures/codegen 仍需 closure |
| C1 Upstream Operational Completion | 未完成 | Admin 仍使用简化 `providerKind` workflow；缺完整 auth/SecretRef、transport、correlation、usage capability、timeouts、结构化 Test/Apply/qualification 体验 |
| C2 Managed Resource Editor Completion | 未完成 | Resources 仍偏 Model-only，存在 `prv_placeholder`；缺 Models/TTS/ASR/MCP/Policy 完整 editor 与 Resource→Upstream→Runtime relationship view |
| C3 Snapshot Projection & Preview | 部分完成 | Hub canonical snapshot/compiler 与 deterministic hash 基础已有；缺新 schema closure、structured Review、canonical Client Snapshot Preview 与同源 executable proof |
| C4 Runtime Reference Profile Completion | 部分完成 | Relay generic SSE/binary/multipart/MCP transport 已有；缺 Test Client/Test Adapter 的四 profile 完整闭环与 required real Adapter qualification |
| C5 Usage / Pricing / Observability Completion | 部分完成 | 后端 ledger/spool/pricing 基础已有；Admin 缺 resource-kind 视角、完整 filters、request detail、UNKNOWN/PARTIAL/cost semantics 与有价值趋势可视化 |
| C6 Browser + Hub + Relay System E2E | 未完成 | 尚无完整 real browser + real Hub/Relay + deterministic Test Client/Test Adapter 的 S0.1 `CAP-*` Gate 证据 |
| C7 Client Contract Freeze Gate | 未开始 | C0–C6 未全部 Green；freeze manifest 尚不存在 |

## 当前实现重点

### Contracts

C0 是当前入口。行为/字段变化先建立 failing contract test，再同步 OpenAPI、fixtures、generated artifacts 和 consumers。Snapshot v1 在 C7 前都视为 pre-freeze。

### Admin Console

具体实现以 `docs/admin-console-implementation.md` 为准；产品/UX 以 architecture 的 Admin Product Requirements 为准。

当前必须完成的产品链：

```text
Upstream / Secret
→ Resources / Policy
→ Relationship View
→ Pricing
→ Validate / Review / Snapshot Preview
→ Publish / Activation
→ Usage / Cost / System
```

### Runtime / Adapter

Relay 保持 provider-agnostic。S0.1 要证明的是 required capability profile 的完整执行链，而不是仅证明某种 HTTP transport 存在。

### Testing / Freeze

C6 使用真实 Admin build、Hub、Relay，加 deterministic Test Client/Test Adapter；real external Adapter qualification 单独形成证据。C7 冻结 manifest 至少 pin：architecture commit、platform-core commit、Client OpenAPI/fixture hash、Snapshot schema version、Admin build identity、qualification reference 和 scenario results。

## 已有基础证据

当前分支历史上已对以下基础形成过可执行 Green evidence：Identity/Enrollment、Draft/Snapshot deterministic behavior、Publish/Relay apply/reconcile、Relay streaming/binary/multipart/MCP transport、usage spool/ingest/dedupe/pricing、SQLite migration/backup、Admin production build 与基础页面 workflow。

这些证据用于回归，不替代新的 C0–C7 Gate；任何新 head 都必须以该 head 实际执行的 CI/测试结果为准。

## 当前不做

- Android S0.2 implementation；
- Agent Space / S1；
- S2/S3；
- Relay WebSocket/realtime tunnel；
- provider-specific body translation in Relay；
- S1+/enterprise 空导航或通用 workflow/dashboard builder。

## 下一步

**C0 Contract Audit & Freeze Preparation**：完成 Client/Admin executable contract closure 与 fixtures/codegen Green，然后按阶段索引依次推进 C1–C7。
