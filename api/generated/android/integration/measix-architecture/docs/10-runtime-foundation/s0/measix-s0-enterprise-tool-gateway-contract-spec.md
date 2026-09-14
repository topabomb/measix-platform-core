# S0.3 — Enterprise Tool Gateway & Governed Tool Integration Contract

> 状态：S0.3 Delivery Contract / 企业工具网关与受治理工具集成基线
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`
> 术语/ID 权威：`../../00-platform/measix-platform-terminology-and-identifier-contract.md`
> Wire 权威：`measix-s0-control-protocol.md`
> 组件架构：`measix-s0-enterprise-tool-gateway.md`
> Admin 产品要求：`measix-s0-admin-console-product-requirements.md`
> 测试权威：`measix-s0-enterprise-tool-gateway-testing-spec.md`
> 下游阶段：`measix-s0-android-integration-contract-spec.md`（S0.4）
> 文档职责：固定 S0.3 的产品范围、两种 Managed MCP 暴露方式、Gateway 工具面、Catalog 生命周期、发布/权限/交互不变量和 Exit；不定义 Go package/file、数据库 DDL、索引库或检索算法实现。

## 1. 阶段目标与核心决策

S0.3 在 S0.2 Realm、Experience 和 Enterprise Update authority 冻结后，增加独立的 **Enterprise Tool Gateway**，用一个固定规模、受 Managed Release 控制的模型工具面承载大量可变企业工具：

```text
Enterprise Realm model context
  ├─ discover_tools
  └─ invoke_tool
          ↓
Runtime Relay
          ↓
Enterprise Tool Gateway
  ├─ Published Tool Catalog
  ├─ platform tool adapter
  └─ downstream MCP client
```

固定决策：

- Enterprise Tool Gateway 是 `control-hub`、`runtime-relay` 之外的第三个服务端 daemon；
- Gateway 对模型只公开 `discover_tools`、`invoke_tool`，名称和顺序固定；
- Tool Catalog、企业 guidance、真实工具 agent-facing metadata 都经 Draft→Validate→Preview→Publish 进入新 `managedGeneration`；
- Gateway 与 Direct Managed MCP 并存，但同一外部工具来源不得在同一 Release 中形成两条模型执行路径；
- S0.3 只交付服务端、Admin 和 Test Client 闭环；Android 真实消费、UI 与设备 Gate 属于 S0.4；
- 检索算法属于 Gateway 内部实现，S0.3 不强制 embedding、向量数据库、Router LLM 或 reranker。

## 2. Entry 与阶段关系

S0.3 Entry：

1. S0.1 当前资源协议、Runtime Relay、Usage 和 security baseline 可重放；
2. S0.2 frozen Snapshot v4、ClientRealm、Managed Assistant/Memory Seed/Starter、Enterprise Update Feed/Portal authority 可重放；
3. 当前 active generation 仍执行 S0 无 grace-window 规则；
4. Gateway 相关 stable ID、Snapshot v5、Gateway control 和错误语义先进入 Architecture/Control Protocol，再进入 executable contract。

S0.3 不宣称 Android 已集成 Gateway。只有 S0.3 Freeze 后，S0.4 才消费 pinned Snapshot v5 和 Gateway surface。

## 3. 两种正式暴露方式

### 3.1 Enterprise Tool Gateway

适合大量、可变、来自多个服务端来源、需要统一审核/过滤/审计的工具。模型顶层只看到两个固定 Meta Tool，真实工具合同按需由 `discover_tools` 返回，再由 `invoke_tool` 代理执行。

Gateway Integration 使用独立 `ToolIntegration`、Candidate/Published Tool Catalog 和 Gateway control state；它不创建 client-visible `McpDefinition`，也不出现在 Android 普通 MCP 管理面。

### 3.2 Direct Managed MCP

适合少量、稳定、Assistant 明确依赖、需要模型直接获得真实 Function Schema 的 MCP Server。它继续使用现有：

```text
McpDefinition
RuntimeBindingDefinition
ManagedAssistantDefinition.mcpServerIds[]
```

Direct Managed MCP 的工具通过标准 MCP `initialize` / `tools/list` 获得，并按既有 Android Managed MCP 规则投影。S0.3 不改变 当前 Direct MCP 的 `McpDefinition` 的含义。

### 3.3 单一执行路径不变量

`ToolIntegration` 只表示 Gateway downstream source，`McpDefinition` 只表示 Direct Managed MCP；二者不是一个带可变 `mode` 的双重实体，也不能在运行时 fallback 到对方。

Validate 必须阻止同一受管来源/工具在同一 Release 中同时 Direct 与 Gateway 暴露。管理员若改变暴露方式，必须显式移除旧发布引用并创建/发布新定义，不得生成过渡期双读、双调用或隐式兼容路径。

## 4. Gateway Profile 与工具面

每个 Deployment 在 S0.3 最多有一个 published Gateway surface：

```text
ToolGatewayProfile
  enabled
  clientEnablementPolicy REQUIRED | USER_CONTROLLABLE_DEFAULT_ON
  discoverGuidance?
  invokeGuidance?
  highlightedGatewayToolIds[]
```

Gateway MCP `tools/list` 必须对当前 authorized generation 返回且只返回：

```text
1. discover_tools
2. invoke_tool
```

同一 generation 内，名称、顺序、description、input/output schema 和 canonical bytes 必须稳定。Gateway 声明工具集合时遵守标准 MCP `tools/list` 语义；Android/Test Client 以 Gateway 的标准 MCP 响应作为实际工具合同，并校验 Snapshot v5 中的 `surfaceHash`，不得绕过 MCP 自行拼接一份平行定义。

`ToolGatewayProfile.enabled=false` 表示管理员没有把 Gateway 纳入该 Release，compiler 因而省略 `ToolGatewayDefinition`；它不是 Android 的运行时开关。Snapshot v5 只发布已纳入 Release 的 Gateway client resource、surface version/hash 和 `clientEnablementPolicy`，不复制两个完整 Tool Definition。Admin Preview、Gateway applied surface 和 Snapshot `surfaceHash` 必须来自同一 canonical compiler。

### 4.1 Enterprise Client 启用政策

只要 Snapshot 含 `ToolGatewayDefinition`，`discover_tools` 与 `invoke_tool` 就是 Enterprise Realm 的内置企业能力，必须原子成对注册、成对关闭，不能逐工具开关，也不绑定某个 Assistant：

```text
REQUIRED
  server 强制开启；客户端不提供可写关闭动作

USER_CONTROLLABLE_DEFAULT_ON
  首次/无本地偏好时开启；用户可以在 Enterprise capability settings 中关闭或重新开启整对工具
```

客户端本地偏好按 `(deploymentId, toolGatewayId)` 隔离，属于 Enterprise-local shadow，不进入 Managed Snapshot、普通 MCP Settings 或跨 Deployment 共享。策略从可选变 REQUIRED 时保留但忽略本地关闭偏好；以后恢复可选时重新显现原偏好。Personal Realm 永不注册 Gateway。

`clientEnablementPolicy` 必填且只能取上述两个值，缺失/未知值使候选 Snapshot 验证失败。本地开关只影响下一次 interaction 捕获的 effective tool set；不能从在途 Tool Loop 中途删除或添加工具。用户若要立即停止，应显式取消当前 interaction；服务端新策略仍通过新 generation 与 Relay barrier 生效。

Direct Managed MCP 仍按 `ManagedAssistantDefinition.mcpServerIds[]` 绑定，不因 Gateway client enablement policy 变成全局默认工具；二者不得 fallback 或共用开关。

## 5. 动态描述的受管组成

工具名称与平台安全合同固定；企业只编辑 guidance segment。最终描述由：

```text
platform invariant contract
+ enterprise guidance
+ compiler-rendered highlighted tools (discover_tools only)
```

组成。

平台不变量至少表达：

- 当前上下文未必包含全部企业能力，需要未知能力时先 discover；
- 复合任务拆成原子能力需求；
- 只使用当前 interaction 返回的 `toolRef` 和 schema；
- 不猜测、修改、跨 interaction 复用工具引用；
- 权限、风险和参数校验由服务端决定。

`discoverGuidance` / `invokeGuidance` 可以表达企业术语、优先顺序、默认业务约定和使用建议，但不能删除、覆盖或与平台不变量冲突。`highlightedGatewayToolIds[]` 只引用同一 Published Catalog 中 enabled 工具，compiler 渲染其 agent-facing name + 一句话 description；悬空、重复或被禁用引用阻止 Publish。

任一 client-visible description、highlighted list、surface schema、client enablement policy 或 Gateway inclusion 状态变化都创建新 Managed Release 和新 `managedGeneration`。Save/Validate/Preview 不生效，不存在脱离 generation 的热提示。

## 6. `discover_tools` 合同

用途：从当前 Principal 在当前 Published Catalog 中有权使用的工具里查找完成任务所需能力，并返回调用所需完整合同。

输入语义：

```text
queries[]          required, 1..5 non-empty atomic capability requests
limitPerQuery?     default 3, range 1..5
```

输出语义：

```text
catalogGeneration
results[]
  query
  matches[]
    toolRef
    gatewayToolId
    name
    description
    inputSchema
    outputSchema?
    risk = READ_ONLY
    expiresAt
```

`catalogGeneration` 等于该请求经 Relay admission 后捕获的 `managedGeneration`，不是第五条独立版本序列。

规则：

- 自然语言、精确 agent-facing name 或 alias 都可以查询；
- 每个 match 直接返回完整、已发布的 `inputSchema`，存在时同时返回 `outputSchema`；
- 不另增 `inspect_tool` / `get_tool_schema`；
- 未发布、disabled、当前 Principal 无权访问或非 `READ_ONLY` 工具绝不进入候选；
- 返回的所有 source text/schema 都必须已被管理员审核并进入 Published Catalog；远端 MCP 的未审核变化不能直接进入模型上下文；
- 结果有界、确定且可解释，但 ranking 算法不进入 wire contract。

S0.3 的授权粒度为 Deployment-wide published enablement + active Enterprise Principal。Organization/Group/RBAC/per-user tool assignment 延后；文档和实现不得假装 S0.3 已有这些粒度。

## 7. `invoke_tool` 与 `toolRef`

输入语义：

```text
toolRef     required opaque String
arguments   required Object
```

不接受任意 tool name、integration ID、endpoint、schema 或客户端自报 risk。

`toolRef` 是 Gateway 签发的短期、不可猜测、interaction-bound capability reference，不是 durable entity ID。其可验证 claims 至少绑定：

```text
deploymentId
userId
deviceId
sessionId
interactionId
managedGeneration
gatewayToolId
schemaHash
expiry
```

Gateway 调用前必须验证签名/完整性、Principal、interaction、generation、expiry、当前 Published Catalog/authorization、schema hash 和 arguments JSON Schema。任一失败不得调用 downstream。

参数约束以已发布 source schema 为准：只有 schema 禁止额外属性时才拒绝额外字段。Catalog validation 必须明确支持的 JSON Schema dialect/keywords；无法验证的 schema 或依赖不受信任远程解析的引用阻止发布，不得静默忽略约束或因 `$ref` 发起任意网络请求。

S0 没有 generation grace window。新 generation 激活后，旧 interaction 的下一次 Relay request 在到达 Gateway 前得到 `managed_snapshot_required` / 428 并终止；Gateway 不通过保留旧 Catalog 绕过 Relay barrier。客户端同步后创建新 interaction，并重新 discover，不自动 replay 旧 invoke。

`toolRef` 的 TTL 必须覆盖正常单次 Tool Loop，但永远不超过其 generation/interaction 的有效性；超时只允许重新 discover，不允许以 tool name fallback。

## 8. Tool Catalog 生命周期

```text
Admin creates Tool Integration
→ configure endpoint + Managed Secret
→ Test Integration
→ Gateway initialize / tools/list
→ Hub stores Raw Candidate Catalog
→ Admin reviews schema/text/annotations and selects READ_ONLY tools
→ edit agent-facing name/description/aliases
→ Validate + Preview
→ Publish
→ immutable Published Tool Catalog @ managedGeneration N
```

远端 `tools/list_changed` 或管理员 Refresh 只产生新 Candidate + diff：added/removed/schema changed/description changed。它不修改当前 Published Catalog，也不推进 generation；只有管理员完成审核并 Publish 后才生效。

每个 Published Gateway Tool 保存：

```text
gatewayToolId
sourceKind = DOWNSTREAM_MCP | PLATFORM
toolIntegrationId?     required only for DOWNSTREAM_MCP
platformToolName?      required only for PLATFORM
sourceName
sourceDescription
sourceInputSchema
sourceOutputSchema?
sourceSchemaHash
agentFacingName
agentDescription
aliases[]
risk = READ_ONLY
enabled
```

两种来源互斥：downstream 工具绑定 `ToolIntegration`；platform 工具绑定平台 allowlist 中的 typed adapter（S0.3 为 `get_enterprise_updates`），不创建虚假的 MCP Integration/endpoint/credential。精确字段和引用规则由 Control Protocol 维护。

S0.3 允许修改 agent-facing name/description/aliases/enabled；不允许管理员改变参数 required/type/validation constraint 或底层调用语义。Agent-facing name 在同一 Published Catalog 内必须唯一；stable identity 是 `gatewayToolId`，不是可修改名称。

MCP tool annotations、description、schema description 和 result 都视为外部输入。S0.3 的 `READ_ONLY` 是 MEASIX 发布政策，不是对第三方服务行为的自动证明；管理员只能发布受信任、经语义资格验证的来源。远端自报 annotation 不得单独决定风险或绕过确认/审计边界。

## 9. 搜索质量

默认实现可以使用：

```text
exact name / exact alias
→ phrase / prefix
→ lexical full-text relevance
→ parameter-name relevance
→ deterministic tie-break by stable ID
```

Gateway 内部以后可增加 BM25、embedding 或 optional rerank，而不改变两个工具合同。S0.3 Freeze 必须用至少 50 个工具的 deterministic evaluation catalog 证明 exact-name 稳定命中、关键工具 Top-K、alias/中英文描述生效、多个 atomic query 分别返回能力，以及未授权/未发布工具永不出现。

## 10. 发布、Control 与恢复

Gateway 参与条件是**当前或目标 Release 含 Gateway**，不是“Gateway 内容字节是否变化”。目标 Release 含 Gateway 时，即使只修改 Model/Assistant，也必须在 Gateway 建立 generation N 对应的 surface/catalog；可复用相同 canonical 内容，但不得遗漏 N 的映射。

目标 Release 含 Gateway 时，新 generation N 的 activation 顺序固定为：

```text
1. Hub validate and persist immutable STAGED Release N
2. Hub compile GatewayControlState G containing Relay-current generation and staged Catalog N
3. Gateway validate + atomic apply G and ACK exact revision/hash
4. Hub compile RuntimeControlState R with active generation N and Gateway resource route
5. Relay validate + atomic apply R and ACK exact revision/hash
6. Hub finalize Release N ACTIVE
```

Gateway-first handoff 期间，applied state 最多同时包含“Relay 当前仍可达的 generation”和“下一 staged generation”。这是切换正确性所需的短暂双版本运行材料，不是客户端 grace window：Gateway 只按 Relay 签名 Principal envelope 选择精确 generation，Relay 切到 N 后所有新的 N-1 请求仍在 Relay 得到 428；已经被 Gateway 接受的旧请求只可在其 captured immutable view 上结束。Hub finalize 后用后续 control revision 清理不可达旧 generation。

目标 Release 移除 Gateway 时，不创建 N 的 Gateway surface/catalog；切换前保留 Relay-current 可达目录，Relay 移除 route 并 ACK 后再清理旧目录。只有当前和目标 Release 均不含 Gateway 时，Publish 才完全跳过 Gateway。Gateway 不可达不阻止这种限制性 route 移除；后续清理失败须可重试/诊断，但不阻塞安全移除后的 finalize，也不得重新开放 public route。

目标含 Gateway 的发布中，Gateway apply 失败时 Relay 不切换；Gateway 成功但 Relay 失败时，新 Catalog 在 Gateway 中保持不可从 public runtime 进入的 dormant state；Hub crash/timeout 通过 Gateway/Relay exact status reconcile，不创建第二个 semantic publish。Gateway 不得用保留的旧 Catalog 接受绕过 Relay barrier 的 public request。

Gateway restart 后没有有效 applied control 时 fail closed，等待 Hub rehydrate。Gateway 不访问 Hub DB，也不把可重建 control state/credential 当成独立 durable authority。

## 11. Runtime 身份与服务边界

公开入口不增加：

```text
/api/admin/v1/*   → Control Hub
/api/client/v1/*  → Control Hub
/runtime/v1/*     → Runtime Relay
```

Android/Test Client 通过 Gateway `toolGatewayId` 的 Runtime resource path 进入 Relay。Relay 完成 auth/principal/generation/resource admission，清除客户端伪造的 internal headers，然后用私有 TLS/service identity 向 Gateway 注入受信 Principal envelope，至少包含 deployment/user/device/session/interaction/generation/request identity。

Relay 不解析 `discover_tools` query、`toolRef`、真实工具名或 arguments。Gateway 不接受 public ingress、Android bearer 直连或客户端自报 Principal。Gateway 调 downstream MCP 时使用 Gateway-owned enterprise credential，不转发 Android access token。

## 12. Platform Tool 与 Enterprise Update

S0.3 把 `get_enterprise_updates` 作为 Published Catalog 中默认 highlighted 的 platform tool：

```text
discover_tools
→ invoke_tool
→ Gateway platform tool adapter
→ Hub private typed read API
→ Enterprise Update authority
```

Control Hub 继续拥有 Draft/Publish/Withdraw、Feed revision/ETag 和 Client/Portal Feed API。Hub 不再提供 MCP initialize/tools/list/tools/call projection，也不建立通用 MCP Server。Gateway→Hub private API 只提供受限只读查询，不把 Admin mutation surface 或 Hub DB 暴露给 Gateway。

## 13. S0.3 范围与非目标

S0.3 只支持：第三个 Gateway daemon、两个固定 Meta Tool、server-published client enablement policy、MCP Streamable HTTP downstream、企业共享 server credential、`READ_ONLY`、Candidate/Review/Publish、platform `get_enterprise_updates`、external MCP Integration、Gateway control/status/reconcile、三个 daemon 的宿主原生进程监管/结构化日志基线、Admin 和 Test Client E2E。

明确延后：write/destructive tools、用户审批、per-user OAuth、Organization/Group/RBAC、per-user assignment、REST/DB/SaaS Connector framework、Code/Shell mode、batch/workflow、必选 vector/LLM router、Gateway HA。Direct Managed MCP 不是 fallback，Phase 2 Connector 也不因 S0.3 Gateway 而提前。

## 14. S0.3 Exit

只有 `measix-s0-enterprise-tool-gateway-testing-spec.md` required evidence 全部 Green，才允许 S0.3 Freeze：

1. Gateway 标准 MCP surface 只有两个固定工具，顺序/schema/description 在 generation 内 byte-stable；
2. Snapshot v5 的 Gateway resource/surfaceHash 与 Gateway `tools/list` exact match，且不复制平行 Tool Definition authority；
3. guidance/highlighted/catalog 只有 Publish 后生效并推进 generation；
4. Candidate drift 不改变 Published Catalog；
5. discover 的完整合同、权限过滤、deterministic search 和 50+ tool context bound 成立；
6. toolRef 不能猜测、跨 Principal/interaction/generation/expiry 使用，arguments 经 authoritative schema 校验；
7. Gateway→Hub platform tool 与 Gateway→external MCP 均通过真实 Hub/Relay/Gateway Test Client topology；
8. Relay 保持 MCP business-transparent，Hub 不进入通用 tool execution path，Gateway 不成为 control authority；
9. Gateway-first / Relay-second activation、restart fail-closed、rehydrate/reconcile 和 Usage/Audit 事实成立；
10. Gateway 两工具按 REQUIRED / USER_CONTROLLABLE_DEFAULT_ON 形成原子、默认开启的 Enterprise built-in surface，Direct Managed MCP 仍保持 Assistant-bound；
11. 三 daemon 生产 supervisor、graceful stop/restart、restart rate-limit 和安全 JSON log collection 的 reference profile 有 executable evidence；
12. Freeze Manifest 固定 architecture/core/build/OpenAPI/fixture/catalog/scenario identities，并明确 Android S0.4 尚未执行。
