# S0 — Foundation Contract 产品与技术基线

> 状态：S0 Umbrella Contract / S0 总体交付基线  
> 版本：2026-08-31
> 上位文档：`../measix-runtime-foundation-architecture.md`  
> 文档导航：`../../measix-documentation-guide.md`  
> 文档职责：定义 S0 最终交付范围、组件边界、S0 Core→S0.1→S0.2→S0.3→S0.4 顺序和最终 Exit；具体 wire、组件内部架构、产品 UX 和测试矩阵由下位 authority 定义。

## 1. S0 权威结构

S0 是 Runtime Foundation 的第一个正式 major Stage。交付顺序固定为：

```text
S0 Core foundation
  ↓
S0.1 Managed Capability Delivery
  ↓
S0.2 Enterprise Realm & Experience Foundation
  ↓
S0.3 Enterprise Tool Gateway & Governed Tool Integration
  ↓
S0.4 Android Managed Runtime Integration
  ↓
S0 Final System / RC Exit
```

`S0.1`、`S0.2`、`S0.3`、`S0.4` 是 S0 内部 delivery sub-stage，不改变 S1/S2/S3 编号。

主要下位 authority：

| 主题 | Authority |
|---|---|
| 平台术语 / stable ID / version role | `measix-platform-terminology-and-identifier-contract.md` |
| 企业经验进化闭环 | `measix-enterprise-experience-lifecycle-architecture.md` |
| S0 全局技术决策与实施依赖 | `measix-s0-implementation-decision.md` |
| S0 跨组件 wire/state/error/security | `measix-s0-control-protocol.md` |
| S0.1 产品闭环 / required profile / Freeze | `measix-s0-capability-delivery-contract-spec.md` |
| S0.1 实施 checkpoint | `measix-s0-capability-delivery-implementation-decision.md` |
| S0.1 pre-Android system gate | `measix-s0-capability-delivery-system-testing-spec.md` |
| Admin 产品与浏览器边界 | `measix-s0-admin-console-product-requirements.md` |
| S0.2 Enterprise Realm / Experience Foundation | `measix-s0-enterprise-realm-experience-contract-spec.md` |
| S0.2 Enterprise Portal MVP | `measix-s0-enterprise-portal-product-requirements.md` |
| S0.2 system/component gate | `measix-s0-enterprise-realm-experience-testing-spec.md` |
| S0.3 Enterprise Tool Gateway | `measix-s0-enterprise-tool-gateway-contract-spec.md` |
| S0.3 Gateway component gate | `measix-s0-enterprise-tool-gateway-testing-spec.md` |
| S0.4 Android integration | `measix-s0-android-integration-contract-spec.md` |
| Control Hub / Runtime Relay / Enterprise Tool Gateway 组件架构 | 对应 component architecture 文档 |
| Upstream Adapter 外部边界 | `measix-s0-upstream-adapter-contract.md` |
| 最终 S0 System / RC | `measix-s0-system-testing-spec.md` |

本文不复制上述文档的完整 ID 表、resource schema、test matrix 或实现结构。

## 2. S0 最终产品闭环

S0 最终必须真实交付：

```text
Admin Console
  → Control Hub: User / Enrollment / Upstream / Secret / Managed Draft
  → Validate / Review / Publish
  → Enterprise Tool Gateway + Runtime Relay apply authoritative desired states
  → Android Enrollment / ClientRealm / preflight / Snapshot
  → /runtime/v1/resources/{resourceId}/...
  → Runtime Relay
  → qualified Upstream Adapter or Enterprise Tool Gateway
  → published platform/downstream tool
  → Usage Ledger / Pricing / Cost / Diagnostics
```

S0.1 证明“服务端已经能生产、执行、计量并冻结可消费的 Managed Capability”；其产品范围是 A 运行资源与 Direct Managed MCP 基线。S0.2 用 Snapshot v4、Enterprise Realm、Managed Assistant/Memory Seed/Assistant Starter、Enterprise Update Feed 与 Portal MVP 建立企业体验基础。S0.3 冻结受治理的大规模工具发现/执行服务端闭环与 Snapshot v5。S0.4 再完成 Android 对 Model/TTS/HTTP-ASR/Direct MCP/Gateway required profile 的完整集成、失败语义和设备 Gate。四个 sub-stage 都不是单独的 S0 Exit。

## 3. S0 组件与仓库边界

S0 有六个逻辑组件和一个外部 Adapter 边界；生产服务端有三个 daemon：

```text
Android Client  → topabomb/rikkahub_mcp
Control Hub     → topabomb/measix-platform-core / control-hub
Runtime Relay   → topabomb/measix-platform-core / runtime-relay
Enterprise Tool Gateway → topabomb/measix-platform-core / enterprise-tool-gateway
Admin Console   → topabomb/measix-platform-core / static SPA
Enterprise Portal → 独立企业前端 / WebView SPA（具体仓库由实施阶段登记）
```

职责边界：

- **Control Hub**：Identity/Enrollment/Session、Draft/Validation/Release/Snapshot、Upstream/Secret、Runtime desired state、Usage/Pricing/Cost 的控制面 authority；不进入大流量 provider data path。
- **Runtime Relay**：执行 Hub 编译的完整 control state，负责 auth/generation/resource/path admission、server-side credential、透明 HTTP transport、request usage spool；不解释 Draft/Release，不做 Provider-specific translation。
- **Enterprise Tool Gateway**：执行 Hub 编译的 Gateway control state，只公开 `discover_tools` / `invoke_tool`，负责 Published Tool Catalog search、toolRef/schema enforcement、platform/downstream MCP 代理与 tool-level facts；不拥有 Release/Identity durable truth。
- **Admin Console**：浏览器管理面，通过公开 Admin API 完成配置/验证/发布/诊断；不拥有 server truth，不直连 Relay internal API。
- **Android Client**：保持 Local Runtime 完整；S0.2 增加 ClientRealm、企业接入、C 类投影和 Portal Native Host，S0.4 完成 Snapshot v5/Effective Runtime 与 required runtime interaction correctness；不成为 thin client。
- **Enterprise Portal**：企业用户工作台前端；S0.2 展示企业状态与企业动态，通过受限 Native Bridge 请求宿主能力；不拥有 Android Local/Managed Store，也不是 Admin Console。
- **Upstream Adapter**：承担 Provider/protocol compatibility；Relay 保持 provider-agnostic。

具体代码目录、class/file、DB DDL、依赖与运行配置名由实现仓库维护。

## 4. S0 稳定跨组件不变量

以下是 S0 必须始终成立的 umbrella invariants；精确字段和 HTTP contract 以 Control Protocol 为准。

### 4.1 Identity 与版本

Stable ID namespace 以 Terminology Contract 为唯一权威。本阶段不复制完整 prefix 表。

四个版本序列角色固定：

```text
managedGeneration     client-visible Release/Snapshot version
controlRevision       Hub → Relay full desired-state revision
gatewayControlRevision Hub → Enterprise Tool Gateway full desired-state revision
managedStateRevision  client control-state change revision
```

四者都不是 entity ID。

### 4.2 Publish

S0 只有：

```text
Publish = activate + enforce
```

Save Draft / Validate 不生效。目标 Release 含 Gateway 时，即使仅 Model/Assistant 改变，也必须先在 Gateway 建立目标 generation 的 surface/catalog 并取得 ACK，再由 Relay apply/ACK、Hub finalize。移除 Gateway 时先让 Relay 安全移除 route，再清理不可达 Gateway state；当前与目标均无 Gateway 才完全跳过 Gateway。精确失败/清理规则以 Control Protocol §6 为准。

S0 不支持 generation grace window；新的 Managed Runtime request 只接受当前 active generation。Rollback 通过 republish 历史内容产生新的 generation，不回退 generation 数值。

### 4.3 Client Resource 与 Server Route 分离

Client 只依赖 client-visible Definition，例如 `resourceId`、`clientProtocol`、resource runtime metadata；不得获得：

```text
runtimeRouteId
upstreamId
Upstream internal/base URL
Enterprise Secret/API key
resolved credential
```

Server 内部解析：

```text
resourceId → RuntimeRoute → active Upstream configuration or private Gateway target
```

### 4.4 Control Plane 与 Runtime Data Plane 分离

- Runtime request path 不同步依赖 Hub DB/CRUD；
- Hub 不转发 Model/TTS/ASR/MCP 大 payload；
- Relay 不读取 Hub DB，也不 import Hub domain/persistence；
- Gateway 承载工具目录/语义/代理执行，Hub 不承载通用 MCP Gateway，Relay 不解析 discover/invoke business body；
- Hub→Gateway、Hub→Relay 分别使用完整 desired-state apply + exact applied status reconciliation；
- Gateway/Relay restart 后没有有效 control state 时必须 fail closed，等待 Hub rehydrate。

### 4.5 Runtime Transport

S0 必须支持：

```text
HTTP request/response
HTTP streaming/SSE passthrough
binary response streaming
multipart upload
cancellation
```

S0 不实现 Runtime WebSocket tunnel、generic path rewrite DSL 或 Provider-specific body translation。

### 4.6 Usage

- Request-level Usage 必须具有稳定 request identity、captured generation 与实际解析的 resource/target correlation；未解析字段和 Gateway 的非 Upstream target 按 Control Protocol 条件表达，不伪造 ID；
- Relay→Hub 短时故障不得在已成功 durable-spool 后静默丢事件；
- provider-specific semantic usage 只有在可靠来源/qualification 下才可声明；
- UNKNOWN/PARTIAL 不得伪造成精确 quantity 或 0 cost。

## 5. S0.1 — Managed Capability Delivery

S0.1 在 Android Enterprise implementation 前完成 server-side product closure：

```text
Admin visual authoring
→ Upstream/Secret operational apply
→ Model/TTS/ASR (A) + MCP (B) + cross-cutting Policy managed definitions
→ canonical Client Snapshot Preview
→ Publish / Relay execution
→ Test Client runtime
→ Usage / Pricing / Diagnostics
→ Client Contract Freeze
```

Required profile、resource information model、Admin UX、Adapter qualification 和 C0–C7 Gate 分别由 S0.1 Contract / Admin Product Requirements / Adapter specs / S0.1 System Testing Spec 定义。

S0.1 不修改 Android Enterprise runtime。它可以读取 Android current reality 作为 contract audit 输入，但不能通过先写 Android 来发明 server fields。

S0.1 Exit 产生 exact candidate Freeze evidence，至少固定 architecture/core/build/client-contract/fixture/schema/adapter/scenario identities。MEASIX 尚未发布，仅保留当前协议；旧原型不构成兼容承诺。候选变化后重新执行受影响门禁，版本权威见 Control Protocol §10.10.1。

## 6. S0.2 — Enterprise Realm & Experience Foundation

S0.2 只有在有效 S0.1 Freeze 后开始。它使用当前唯一 `schemaVersion=4`，并建立第一个企业用户可感知的 Realm/Experience 基础：

```text
Enrollment → Enterprise Realm
           → Managed Chat Model (A)
           → Enterprise Update Feed / Portal capability (B)
           → Managed Assistant + mature Memory Seed + Assistant Starter (C)
           → existing Android chat/experience boundaries
           → Enterprise Portal status + updates list/detail
```

核心规则：

- Personal Realm 完全不展示、不可选择、不可执行任何企业下发内容；
- Enterprise Realm 按 Policy 使用 Managed 内容及获准的用户原配置/凭据；不复制企业资源定义，不共享 Personal 运行数据，详细准入见生命周期架构 §4；
- 当前 Snapshot v4 包含 Managed Assistant 与 Assistant Starter；旧版本样例不再保留；
- Managed Memory Seed 是 generation-bound、只读的企业成熟经验，不写入 Android 可变 `MemoryEntity`；
- Assistant Starter 只预填可编辑提示词，不 auto-send、不自动调用工具；
- 企业动态是独立 Feed，不进入 Managed Snapshot，也不改变 `managedGeneration`；
- Enterprise Update Feed 在日期参数均省略时按 `limit` 返回最新内容；日期过滤保持可选；
- Portal 使用短期受限 Web Session 与 allowlisted Native Bridge，不接触 Refresh Credential 或 Android Store；
- 首次接入、应用启动/回前台、用户手动刷新、会话续期后和 generation mismatch 构成明确同步触发点。

精确对象、投影、Session、Portal 与测试要求由 Enterprise Realm & Experience Contract、Portal Product Requirements 和对应 Testing Spec 定义。

## 7. S0.3 — Enterprise Tool Gateway & Governed Tool Integration

S0.3 在 S0.2 Freeze 后新增第三个服务端 daemon，冻结一个 constant-size、generation-bound 的受治理企业工具面：

```text
Published Tool Catalog
→ discover_tools returns reviewed complete tool contracts + short-lived toolRef
→ invoke_tool validates Principal/interaction/generation/schema
→ platform tool or downstream MCP
```

Gateway 与 Direct Managed MCP 是两个独立 authority；同一来源不允许双路径暴露。Snapshot v5 只增加 Gateway logical resource + expected surfaceHash + client enablement policy，客户端仍通过标准 MCP `tools/list` 获得两个工具合同。两个 Gateway 工具是原子、默认开启的 Enterprise built-in pair；服务端决定 REQUIRED 或允许用户成对关闭，Direct Managed MCP 仍保持 Assistant-bound。S0.3 同时冻结三 daemon 的宿主原生生产监管和结构化日志收集基线，只以 Admin + Test Client 证明服务端闭环，不宣称 Android 已集成。精确范围与 Gate 由 Enterprise Tool Gateway Contract / Component Architecture / Testing Spec 定义。

## 8. S0.4 — Android Managed Runtime Integration

S0.4 在 S0.3 Freeze 后完成 Android 对 Snapshot v5 required profile 的完整运行时接入：

```text
PERSONAL   → Built-in + Personal Local → existing Local Runtime

ENTERPRISE → Applied Managed Snapshot v5
           + policy-allowed User Configuration + Enterprise Preferences
           → immutable effective-runtime context
           → existing Model/TTS/HTTP-ASR/Direct MCP execution boundaries
           → Enterprise Tool Gateway discover/invoke boundary
           → Runtime Relay
```

S0.4 保持 Local/Managed state 分离、realm ownership、generation correctness、428/revoke/restart/cancel 语义和企业凭据边界。Gateway 不进入普通 MCP Settings；真实工具动作仍必须可见。它不重新定义 S0.2/S0.3 的 Assistant、Starter、Enterprise Update 或 Catalog 产品语义，只消费已冻结结果并完成全 required runtime profile。精确边界与 Gate 由 Android Integration Contract / Android Client Testing Spec 定义。

## 9. S0 非目标

S0 不实现：

- Agent Space、Agent Runtime、Remote Agent、Runtime Hook；
- Agent Fleet、通用 REST/DB/SaaS Enterprise Connector framework、User Sync；
- Managed Skill、企业助手记忆回传、Experience Contribution 提炼/审核/再发布；
- Enterprise Portal 在 MVP 之外的同步、上传备份、经验库、任意文件选择和媒体上传业务；S0.2 的受限拍照/录音及当前页面预览属于交付范围；
- 企业通知、定向活动/起点对话投递、任务和阅读回执；
- Organization/Group/RBAC、User/Group Capability Assignment；
- 多 Control Hub HA / consensus；
- 通用 workflow/scheduler/message broker；
- generic Quota Engine；
- SaaS Billing / invoice；
- Provider-native protocol translation in Relay；
- Runtime WebSocket/Reatime tunnel；
- 未经 contract + qualification 定义的 provider/profile 扩张。
- Gateway write/destructive tools、用户审批、per-user OAuth/RBAC、Code/Shell/Batch workflow、HA 和必选向量/Router LLM。

不要为了未来阶段提前增加空 abstraction、空导航或占位服务。

## 10. 最终 S0 Exit

只有 `measix-s0-system-testing-spec.md` 定义的最终 T4/RC Gate 全部 Green，且 evidence 固定 exact architecture/core/android/build/contract/adapter identities，才允许宣称 S0 Exit。

最终必须至少证明：

1. Admin 在 clean environment 通过 public UI 完成 required authoring→publish；
2. Hub/Gateway/Relay publish、security、recovery 和 usage/audit invariants 成立；
3. required Adapter profile 有对应 real qualification evidence；
4. Android 从 Enrollment 开始建立 Personal/Enterprise ClientRealm，真实消费 frozen Snapshot v5，并通过 Runtime Relay 执行 required Model/TTS/ASR/Direct MCP/Gateway；
5. Managed Assistant/Memory Seed/Assistant Starter、Enterprise Update Gateway tool 与 Portal 列表/详情形成可验证的 A/B/C 企业闭环；
6. generation/revoke/restart/cancel/usage correlation 等 failure paths 不破坏 S0 不变量；
7. 任何客户端都不能借 internal topology、Secret 或 server route 绕过平台边界。
