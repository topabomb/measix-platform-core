# S0.1 Platform Core 进展

> Architecture baseline: `topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`  
> 当前阶段：**S0.1 Managed Capability Delivery**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

S0 Core 基础已建立：Identity/Enrollment、Draft/Release/Snapshot、Relay desired-state apply/admission/transport、Usage spool/ledger/pricing、Admin shell。

**S0.1 尚未完成。** Client Control/Snapshot v1 仍是 pre-freeze。不能进入 S0.2 Android，不能宣称 S0 Exit。

Hub 侧与 Relay 侧全部 MUST 测试场景已覆盖并验证 Green（见 `docs/s0-review-report.md`）。

## C0–C7 状态

| Checkpoint | 状态 | 当前主要缺口 |
|---|---|---|
| C0 Contract Audit & Freeze Prep | 进行中 | TTS `voice` 与 MCP `authOwnership` 已补齐；Client/Admin OpenAPI 其余字段/enums、ASR managed semantics、usage/preview contract 仍需 closure |
| C1 Upstream Operational | 未完成 | Admin 仍使用简化 `providerKind` workflow；缺完整 auth/SecretRef、transport、correlation、usage capability、timeouts、结构化 Test/Apply |
| C2 Managed Resource Editor | 未完成 | Resources 偏 Model-only，存在 `prv_placeholder`；缺 Models/TTS/ASR/MCP/Policy 完整 editor 与 Resource→Upstream→Runtime relationship view |
| C3 Snapshot Projection & Preview | 部分完成 | Hub canonical snapshot/compiler 与 deterministic hash 基础已有；缺新 schema closure、structured Review、canonical Client Snapshot Preview |
| C4 Runtime Reference Profile | 部分完成 | Relay generic SSE/binary/multipart/MCP transport 已有；缺 Test Client/Test Adapter 四 profile 完整闭环 |
| C5 Usage / Pricing / Observability | 部分完成 | 后端 ledger/spool/pricing 基础已有；Admin 缺 resource-kind 视角、完整 filters、UNKNOWN/PARTIAL/cost semantics 与趋势可视化 |
| C6 Browser + Hub + Relay System E2E | 未完成 | 尚无 real browser + real Hub/Relay + deterministic Test Client/Test Adapter 的 S0.1 Gate 证据 |
| C7 Client Contract Freeze Gate | 未开始 | C0–C6 未全部 Green；freeze manifest 不存在 |

## 契约与管理

- **契约优先**：行为/字段变化先建立 failing contract test，再同步 OpenAPI → fixtures → codegen → 实现。Snapshot v1 在 C7 前视为 pre-freeze。
- **Admin Console**：实现以 `docs/admin-console-implementation.md` 为准；产品/UX 以 architecture 的 Admin Product Requirements 为准。
- **Runtime / Adapter**：Relay 保持 provider-agnostic。S0.1 证明 required capability profile 的完整执行链。
- **Testing / Freeze**：C6 使用真实 Admin build、Hub、Relay + deterministic Test Client/Test Adapter。C7 freeze manifest 至少 pin：architecture commit、platform-core commit、Client OpenAPI/fixture hash、Snapshot schema version、Admin build identity、qualification reference 和 scenario results。

## 当前不做

- Android S0.2 implementation
- Agent Space / S1
- S2/S3
- Relay WebSocket/realtime tunnel
- provider-specific body translation in Relay
- S1+/enterprise 空导航或通用 workflow/dashboard builder

## 下一步

**C0 Contract Audit & Freeze Preparation**：完成 Client/Admin executable contract closure 与 fixtures/codegen Green，然后按阶段索引依次推进 C1–C7。
