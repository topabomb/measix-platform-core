# S0 / S0.1 Platform Core 开发计划与当前进展

> 架构权威：`topabomb/measix-architecture@main`。  
> 当前架构基线：`6de9bfb794e60e9bb6c62501263cc1518e4f5ee3`（S0.1/S0.2 delivery sub-stage + Admin Console Product/UX Requirements 已合并）。  
> 当前交付阶段：**S0.1 Managed Capability Delivery**。  
> 本仓库范围：`measix-platform-core`（Control Hub + Runtime Relay + Admin Console + executable contracts/tests）；S0.2 Android 实现在 `rikkahub_mcp`，S1 Agent Space 不属于当前阶段。

## 1. 当前结论

当前代码不能再描述为“platform-core I0–I5 已完成，只剩 Android/最终 RC”。架构权威把 S0 的实际交付顺序固定为：

```text
S0 Core foundation
  → S0.1 Managed Capability Delivery
  → S0.2 Android Managed Runtime Integration
  → S0 Exit
```

现有分支已经具备大量 S0 Core 后端基础，但 **S0.1 尚未完成，Client Snapshot/OpenAPI v1 仍是 pre-freeze，当前不能进入 Android S0.2 实现，也不能宣称 S0 Exit。**

S0.1 的目标不是继续增加后端 CRUD，而是把现有基础收敛成真实可用的产品闭环：

```text
Admin Console
  → Upstream / Secret
  → Managed Model / TTS / ASR / MCP / Policy
  → Relationship View
  → Validate / Review / Client Snapshot Preview
  → Publish
  → Runtime Relay
  → deterministic / qualified Upstream Adapter
  → Usage / Pricing / Cost / Diagnostics
  → Client Contract Freeze Manifest
```

Admin Console 现在有独立的 architecture Product/UX authority：`measix-s0-admin-console-product-requirements.md`；本仓库的具体 Vue/Quasar/依赖/组件实现以 `docs/admin-console-implementation.md` 为权威。

语义、required profile、字段、产品体验和 Gate 以 `measix-architecture` 的 S0.1 文档为唯一上位权威；本文只记录本仓库的实现状态和证据。

## 2. 执行规则

- 行为修改继续严格执行 `Red → Green → Refactor`；
- docs-only 同步不制造人工 Red，但最新提交仍必须经过仓库 CI；
- OpenAPI/fixture/codegen 修改必须同提交保持 generated drift Green；
- 任何 Android-visible semantic change 必须先有 architecture authority；
- Admin required workflow/information architecture/UX Exit 改变必须先更新 architecture Product Requirements；
- 正常 Vue component、chart/topology/date/helper package 选择属于本仓库 concrete implementation，不要求 architecture npm 白名单；
- Runtime Relay 保持 provider-agnostic，不加入 OpenAI/Anthropic/Google body translation；
- S0.1 Freeze Gate 之前，`client-control.openapi.yaml`/Snapshot v1 视为 pre-freeze；
- S0.1 Freeze Gate 之后，不允许静默破坏已冻结 v1 后再要求 Android 跟随 latest。

## 3. 已有 S0 Core 基础

旧 I0–I5 仍用于追踪历史工作流和回归证据，但不再代表当前交付顺序。

| 旧工作流 | 当前实现判断 | 说明 |
|---|---|---|
| I0 Engineering & Executable Contracts | 基础已完成 | 四份 OpenAPI、fixtures、codegen、platformid、SQLite/Ent/Atlas、Hub/Relay health、Quasar/CI 基础已存在；但 Client OpenAPI 仍需按 S0.1 authority 完成 pre-freeze schema closure |
| I1 Identity & Enrollment | 核心已完成 | User/Enrollment/Device/Session/Admin auth/revoke 等已有实现和测试；继续作为 S0.1 regression baseline |
| I2 Draft & Snapshot | **核心 domain 已有，S0.1 产品契约未完成** | Draft/Snapshot compiler、reference validation、deterministic hash 已有；Android-visible resource fields/enums、Admin 完整资源编辑、Review/Snapshot Preview 仍缺 |
| I3 Relay & Publish | 核心已完成 | desired-state apply、ACK/finalize、reconcile、generation admission、routing/security 已有；继续作为 S0.1 runtime foundation |
| I4 Relay Transport | Relay 侧基础已完成 | SSE/binary/multipart/MCP Streamable HTTP/cancel 等已有测试；这不等于四类 S0.1 required profile 已完成真实产品闭环，也不等于 Android I4/S0.2 |
| I5 Metering/Admin/Hardening | **后端核心已有，产品闭环未完成** | spool/ingest/dedupe/pricing/DB/security 等已有；Admin Usage/Resources/Upstreams/Preview/relationship/filters 与 S0.1 browser/system gate 仍明显不足 |

历史 Green 证据仍有效作为回归基线，例如：

- Identity/Enrollment：`TestSYSI1001IdentityHTTPClosedLoop`、`TestI1IdentityEnrollmentRefreshAndRevoke`、Admin cookie/CSRF tests；
- Draft/Snapshot：`TestHUBCAP001*`、`TestHUBCAP006SnapshotDeterministicAndClientSafe`、`TestHUBCAP008StagedReleaseIsImmutable`；
- Publish/Relay：`TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck`、`TestI3ControlApplyAndRuntimeAdmission`、reconcile/conflict tests；
- Relay transport：`TestRLYI4TransportsStreamWithoutProtocolTranslation`、cancellation/in-flight/routing/header tests；
- Metering：Relay spool、Hub requestId dedupe、semantic completeness、pricing、security revoke、DB backup/restore tests。

这些测试证明已有基础，不替代新的 S0.1 `CAP-*` Gate。

## 4. S0.1 当前差距审查

### C0 — Contract Audit & Freeze Preparation — **未完成**

当前 `api/client/client-control.openapi.yaml` 与 architecture authority 仍有明确差距：

- `ProviderDefinition.clientProtocol` 仍是自由字符串；
- Model `inputModalities/outputModalities/capabilities` 仍是自由字符串数组，尚未形成冻结 vocabulary/schema；
- `TtsDefinition` 尚未表达 architecture 已要求的完整 Android-visible TTS profile（包括 `voice`）；
- `AsrDefinition` 尚未完成 S0.1 managed HTTP transcription 所需的冻结字段；
- `McpDefinition` 尚未完成 S0.1 managed MCP auth ownership contract；
- canonical Snapshot fixtures/hash golden 仍需随新 schema 同步；
- Admin/OpenAPI usage query/preview contract 仍需按 S0.1 authority 完成。

因此当前 `schemaVersion=1` **只能视为 pre-freeze**。

### C1 — Upstream Operational Completion — **未完成**

后端已有 Upstream/Secret/candidate-active revision/Apply 基础，但当前 `UpstreamsPage.vue` 仍存在明显产品缺口：

- Create 表单只提交 `name/baseUrl/providerKind`；
- `providerKind` 是旧 UI 概念，不是当前正式 Upstream executable config 的核心字段；
- transport capabilities、auth/SecretRef、correlation mode、usage capability、timeouts 未形成完整可编辑 workflow；
- Test 结果没有形成结构化 verification 体验；
- Qualification/profile verification 没有形成 S0.1 真实操作与证据闭环。

C1 必须让管理员不用 JSON/DB/内部 API 就能建立真正可运行的 Upstream。

### C2 — Managed Resource Editor Completion — **未完成**

当前 `ResourcesPage.vue` 实质仍是 Model-only Draft editor：

- “Add model” 使用 `prv_placeholder`；
- 没有完整 Provider editor/selector；
- 没有正式 Models / TTS / ASR / MCP / Policy 五类编辑 workflow；
- 没有完整 Upstream binding/transport/path/method 配置体验；
- 没有 architecture Product Requirements 要求的 Resource→Upstream→Runtime relationship view；
- 不能通过现代、可理解的 UI 组织 architecture 要求的四类受管能力。

因此旧进度中“ResourcesPage 已实现”只能解释为页面骨架/基础 Draft workflow 已实现，不能解释为 S0.1 Resource product completion。

### C3 — Snapshot Projection & Preview — **部分完成**

已有 Hub Snapshot compiler、deterministic hash、Client-safe projection 基础，这是可复用的重要资产。

仍缺：

- S0.1 新字段/schema closure；
- Admin structured Review flow；
- Admin “Client will receive” Snapshot Preview；
- Preview 与 Release Snapshot 必须调用同一 canonical projection/compiler 的 executable proof；
- 明确展示不会下发 Android 的 Upstream/Secret/RuntimeRoute/Pricing 等内部数据；
- Client OpenAPI/fixture hash freeze evidence。

### C4 — Runtime Reference Profile Completion — **部分完成**

Relay generic transport 已具备较完整基础，但 S0.1 需要证明的不是“Relay 支持某 transport”，而是 required Managed Capability profile 的完整执行链。

仍需：

- deterministic Test Client 使用真实 Client Control/Runtime public API；
- deterministic Test Adapter 覆盖 required profile、错误/no-forward/cancel/usage correlation；
- Model streaming、TTS binary、ASR multipart、MCP Streamable HTTP 四类 profile 从 Published Snapshot 到 Runtime 的完整闭环；
- required real Adapter/profile qualification；
- qualification 证据按 adapter version + config revision + profile + transport + usage capability 记录，而不是模糊的“支持 S0”。

### C5 — Usage / Pricing / Observability Completion — **部分完成**

后端 Usage Ledger/spool/dedupe/pricing 已有基础，但当前 Admin 产品面仍不足：

- `UsagePage.vue` 主要只有时间范围、Requests/Bytes/Cost/semantic meter chips；
- request row 仍使用 `modelId` 语义，未正确围绕通用 `resourceId/resource kind` 组织；
- 缺 architecture 要求的 User/Resource/Resource Kind/Upstream/Status/Completeness 等主要过滤；
- Model/TTS/ASR/MCP 的 semantic meter/Unknown/Partial/Cost 观察不完整；
- 缺少有决策价值的 Usage/Overview 趋势/比较可视化；
- Pricing 编辑和 request detail/correlation 仍需形成管理员可用闭环。

### C6 — Browser + Hub + Relay + Test Adapter/Test Client E2E — **未完成**

已有本地 Hub/Relay/Console 启动链和组件测试，但没有证据证明 architecture S0.1 全套 `CAP-*` 场景已经通过真实浏览器 + real Hub/Relay + deterministic Test Client/Test Adapter 的完整拓扑。

Admin browser flow 还必须覆盖 Product Requirements 的 loading/empty/error/degraded、relationship view、Review/Preview、Apply/Publish recovery 和 Usage/Pricing 关键交互。

### C7 — S0.1 Client Contract Freeze Gate — **未开始**

必须在 C0–C6 全部 Green 后才生成 freeze manifest。当前不存在可作为 S0.2 输入的有效 manifest，因此 Android 不应以当前 Client OpenAPI 作为“已经冻结的 v1”开始实现。

## 5. S0.1 执行顺序

严格按 architecture `measix-s0-capability-delivery-implementation-decision.md`：

```text
C0 Contract Audit & Freeze Preparation
  ↓
C1 Upstream Operational Completion
  ↓
C2 Managed Resource Editor Completion
  ↓
C3 Snapshot Projection & Preview
  ↓
C4 Runtime Reference Profile Completion
  ↓
C5 Usage / Pricing / Observability Completion
  ↓
C6 Browser + Hub + Relay + Test Adapter/Test Client E2E
  ↓
C7 S0.1 Client Contract Freeze Gate
```

允许在实现层做不破坏依赖关系的并行工作，但 Gate 不得倒置：尤其不能跳过 C0 的 Client contract closure，也不能在 C7 前把 Android integration 当作当前实现目标。

Admin 前端内部实施顺序由 `docs/admin-console-implementation.md` 进一步细化为：shared primitives → Upstream → Resources → relationship → Review/Preview/Publish → Pricing → Usage/Overview/System visualization → browser E2E hardening。

## 6. S0.1 Exit 必须产出的本仓库证据

至少包括：

- 最新 architecture baseline commit；
- `platformCoreCommit`；
- Client Control OpenAPI hash；
- canonical fixture hash；
- frozen Snapshot schema version；
- Admin production build identity/hash；
- required real Adapter qualification reference；
- S0.1 `CAP-*` scenario results；
- deterministic four-profile runtime evidence；
- no-forward/security/failure/usage evidence。

只有这些证据齐全且 Gate Green，才能把 Client contract 标记为 frozen 并进入 S0.2。

## 7. 当前不做

S0.1 不修改：

- `rikkahub_mcp` Android implementation；
- `weero-agent-space`；
- S1 Agent Space；
- S2 Agent Runtime；
- S3 Runtime Hook；
- Phase 2 RBAC/Group/SSO/User Sync/Quota platform；
- Runtime Relay WebSocket/realtime tunnel；
- provider-specific body translation in Relay；
- drag-and-drop workflow builder/custom dashboard designer 等 Product Requirements 明确不属于 S0.1 的管理平台功能。

S0.2 Android 接入只在 C7 freeze manifest 完成后启动。

## 8. 已知工程验证缺口

历史进度中已确认但尚不能伪报完成的项目继续保留：

- Go race detector 尚未形成当前 required CI lane 的已观察 Green 证据；
- S0.1 real browser full flow 未执行完整 Gate；
- required real Adapter qualification 未完成；
- S0.1 Client Contract Freeze manifest 未生成；
- final Android emulator/device T4 属 S0.2/最终 S0，不在当前 S0.1 阶段执行。

## 9. 历史 CI / 本地基础证据

此前分支已出现过完整 PR aggregate Green，例如：

- run `32273718821`：static-contract + backend-test + console-test + aggregate Green；
- run `32274912399`：前端页面骨架/生产构建相关变更后 aggregate Green；
- 本地曾验证 `go build ./...`、`go vet ./...`、`go test ./... -count=1`、Quasar typecheck/test/build、Hub+Relay+Console dev startup 与 Admin login。

这些是已有基础的历史证据。**每次后续 S0.1 行为/contract 变更仍必须以最新 head 的实际 CI/测试结果为准，旧 Green 不能替代新 head。**

## 10. 下一步

当前唯一正确的开发入口是 **C0 Contract Audit & Freeze Preparation**：

1. 按 architecture commit `6de9bfb794e60e9bb6c62501263cc1518e4f5ee3` 对 Client/Admin executable contract 建立 failing contract tests；
2. 修正 Client Snapshot/OpenAPI/fixtures/generated artifacts；
3. 让新的 Android-visible contract 在 core 内部达到确定、可测试状态；
4. 后续 C1/C2 的 Admin 实现同时遵循 architecture Product Requirements 与 `docs/admin-console-implementation.md`；
5. 然后依次推进 C1–C7。

在 C7 完成前，不再使用“platform-core S0 已完成”“可以直接开始 Android”之类表述。
