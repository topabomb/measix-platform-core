# Runtime Foundation（Phase 1）总体架构设计

> 状态：Phase 1 Architecture Baseline / Phase 1 架构基线  
> 版本：2026-08-31
> 上位文档：`measix-agent-platform-roadmap.md`  
> 术语/ID 权威：`measix-platform-terminology-and-identifier-contract.md`  
> 企业经验生命周期：`measix-enterprise-experience-lifecycle-architecture.md`
> S0 实施基线：`measix-s0-foundation-contract-spec.md`  
> 文档职责：定义 Phase 1 的系统边界、项目组织、核心领域、Control/Runtime/Metering 关系、S0–S3 演进和 Android 集成边界；不复制 S0 的完整 REST schema、数据库 DDL 和单组件内部类实现。

## 1. Phase 1 目标与完成定义

Runtime Foundation 面向单企业自托管部署，在保持 Android 完整本地 Runtime 的前提下建立企业运行基础。

Phase 1 结束时必须具备：

1. 稳定 EnterpriseUser / Device / Session / Principal；
2. Personal/Enterprise ClientRealm 及 Capability/Policy/Release/Snapshot 的受管交付基础；
3. Control Hub 发布并强制客户端采用新配置；
4. Runtime Relay 统一承担企业 Runtime admission 与透明转发；
5. Enterprise Tool Gateway 用固定 discover/invoke surface 承载受治理企业工具；
6. Model/TTS/ASR/Direct MCP 经 Upstream Adapter、Gateway tool 经 platform/downstream MCP 使用企业资源；
7. append-only Usage Ledger 与基础 Cost；
8. 每用户持久、隔离 Agent Space；
9. Agent Runtime 原语和 request-scoped Remote Delegation；
10. Runtime Hook / Conversation Intervention。

Phase 1 明确不实现：多租户、复杂 Organization/Group/RBAC、Conversation 双向云同步、SaaS Billing、跨区域 HA、durable Agent Fleet、通用 Enterprise Connector framework、完整 A2A/Federation。

### 1.1 Phase 1 的项目边界原则

- `Control Hub`、`Runtime Relay`、`Enterprise Tool Gateway` 是三个独立故障域；
- `Admin Console` 是前端组件，不独立成为后台服务；
- `Agent Space` 是独立 Runtime 资源服务；
- `Agent Runtime` 在 S2 先作为逻辑边界，允许与 Agent Space 同进程实现；
- `Remote Agent` 是 delegation role，不是另一套执行服务；
- `Agent Fleet` 延后到 Phase 2，避免把 durable scheduler/queue/checkpoint 提前塞入 Phase 1。

## 2. Phase 1 总体架构

```text
Enterprise Admin
      │
Admin Console
      │  /api/admin/v1/*
      ▼
┌──────────────────────────────────────┐
│ Control Hub                          │
│ Identity / Capability Catalog       │
│ Policy / Draft / Release / Snapshot │
│ Runtime Desired State               │
│ Usage Ledger / Cost                 │
└───────────────┬──────────────────────┘
                │ private Relay/Gateway control + usage contracts
                ▼
┌──────────────────────────────────────┐
│ Runtime Relay                        │
│ Auth / Principal                    │
│ Generation Barrier                  │
│ Resource Authorization              │
│ Route / Credential / L7 Relay       │
│ Request Metering                     │
└───────────┬──────────────────────────┘
            ├─ Upstream Adapter → AI/Speech/Direct MCP Provider
            ├─ Enterprise Tool Gateway → discover/invoke → platform/downstream MCP
            └─ Agent Space (S1+) → Agent Runtime (S2) → Runtime Hook (S3)

Android Client
   │ /api/client/v1/* → Control Hub
   └ /runtime/v1/*    → Runtime Relay

S0.2 Enterprise Portal MVP
   │ user-facing enterprise workflow
   └ Control Hub / restricted Android native bridge（User Sync 属于 Phase 2）
```

对外统一入口：

```text
https://example.company.com
  /admin/*          → Admin Console static assets
  /api/admin/v1/*   → Control Hub Admin API
  /api/client/v1/*  → Control Hub Client Control API
  /runtime/v1/*     → Runtime Relay
  <S1 space path>   → Agent Space access surface
```

Android 只保存一个 `platformUrl`；组件拓扑和内部地址由部署侧管理。

### 2.1 Integration Boundary

```text
Runtime Relay → Upstream Adapter → AI/Speech/MCP Provider
Runtime Relay → Enterprise Tool Gateway → platform tool / downstream MCP  # S0.3
Agent Runtime → Enterprise Connector → ERP/CRM/DB/SaaS   # Phase 2
```

三类边界不合并：Provider protocol compatibility、S0.3 governed MCP Tool Integration 与 Phase 2 多协议企业业务系统 integration 是不同问题。

## 3. Phase 1 项目组织与正式名称

### 3.1 Android Client — `rikkahub_mcp`

保留现有本地 Chat/Assistant/MCP/Workspace/Tool Loop，并增加 Enterprise Binding、Snapshot、Effective Runtime、Remote Delegation 和 Hook 接入。它不保存企业共享 Secret，也不做企业管理。

S0 规格：`measix-s0-android-integration-contract-spec.md`。

### 3.2 Control Hub — `control-hub`

模块化单体，是控制面权威中心。以下是逻辑职责，不规定 package/directory：

```text
Identity / Capability / Experience / Enterprise Updates
Policy & Release / Upstream / Runtime Control / Usage
Admin API / Client API / optional Admin static host
```

不承载 Model/TTS/ASR/通用 MCP 大流量 payload。Enterprise Update authority 只向 Gateway 提供窄范围 private typed read API，不在 Hub 内提供 MCP initialize/list/call projection。S0 规格：`measix-s0-control-hub.md`。

### 3.3 Runtime Relay — `runtime-relay`

独立进程，定位为 **identity-aware, policy-aware L7 runtime relay**：Authenticate、Principal、generation barrier、resource authorization、route、credential、transparent transport、cancellation、correlation 和 request metering。

`relay` 比 `gateway` 更准确：本组件不做 Provider body translation，也不是通用 API gateway。S0 规格：`measix-s0-runtime-relay.md`。

### 3.4 Enterprise Tool Gateway — `enterprise-tool-gateway`

S0.3 新增独立进程，只在 Enterprise Realm 提供固定 `discover_tools` / `invoke_tool` MCP surface，消费 Hub Published Tool Catalog/Gateway control，完成搜索、toolRef/schema enforcement、platform/downstream MCP 代理和 tool-level facts。它不访问 Hub DB、不拥有 Release/Identity authority，也不进入 public ingress。

### 3.5 Admin Console

Vue/Quasar SPA，只调用 Control Hub Admin API；S0 构建产物可由 Control Hub/Ingress 静态托管，不形成独立后台进程。

### 3.6 Enterprise Portal

独立 WebView SPA，是企业用户工作台而不是 Admin Console。S0.2 交付 Android Native Host、企业状态/操作、企业动态列表/详情及受限拍照/录音；Portal 不拥有 Android Local/Managed state，通过受限 Web Session 和 allowlisted Native Bridge 使用宿主能力。手机能力的权限、取消、结果归属及设备验收由 S0.2 合同和 Portal 产品要求约束。

### 3.7 Upstream Adapter — 外部依赖

CLIProxyAPI、LiteLLM、New API、专用 Speech Gateway 或企业内部推理服务。负责 Provider protocol/auth/selection 和可用时的 semantic usage。

### 3.8 Agent Space — `weero-agent-space`（S1）

拥有独立 VM/disk/runtime lifecycle、存储/计算 quota 和 WebDAV/MCP access surface。每个用户空间是独立 disk，不得退化为共享宿主目录子目录。

### 3.9 Agent Runtime（S2）

执行 AgentDefinition/AgentRun 的逻辑子系统。S2 初版允许与 Agent Space 同进程/同仓库模块化实现；Phase 2 Agent Fleet 出现后再决定是否拆独立服务。

### 3.10 Runtime Hook（S3）

运行介入机制，不是独立通用 workflow engine；最初由 Android/Agent Runtime 在固定 hook point 调用。

## 4. Stable Plane 与职责

### 4.1 Identity & Control Plane

Control Hub 回答“谁、允许什么、当前发布什么、Runtime 应执行什么”。拥有：

```text
Deployment / User / Device / Session
Capability Catalog / Policy
Draft / Release / Snapshot / Managed State
Upstream / Secret / RuntimeRoute
Agent Catalog (S2+)
Runtime Desired State
Usage Ledger / Pricing
```

不拥有 AgentSpace、AgentRun、AgentInstance 等运行实例。

### 4.2 Runtime Plane

```text
S0  Runtime Relay
S0.3 Enterprise Tool Gateway
S1  Agent Space
S2  Agent Runtime / AgentRun
S3  Runtime Hook Invocation
```

Runtime Plane 消费已发布 Definition/Policy 和 Principal，不直接读取 Android Local Settings。

### 4.3 Integration Boundary

- Upstream Adapter：AI/Speech/MCP Provider compatibility；
- Tool Integration（S0.3）：受治理的 READ_ONLY downstream MCP tool source；
- Enterprise Connector（Phase 2）：ERP/CRM/DB/SaaS data/action integration。

### 4.4 Metering

S0/Phase 1 统一进入 Usage Ledger：Relay Request Usage、Gateway Tool Execution、Upstream Semantic Usage、Agent Space Usage、AgentRun Usage、Hook Usage。不同粒度通过 correlation 关联，不重复计数。Usage 模块保留在 Control Hub 内，不单独拆服务。

## 5. Identity 与 Principal 架构

### 5.1 最小身份模型

Phase 1 固定：

```text
EnterpriseDeployment
  id dep_*

EnterpriseUser
  id usr_*

Device
  id dev_*
  ownerUserId

EnterpriseSession
  id ses_*
  userId
  deviceId
```

Android 通过 Enrollment 建立 Device ↔ EnterpriseUser 绑定，之后业务请求从 Enterprise Session 解析身份，客户端不得自报任意 `userId` 获取资源。

### 5.2 PrincipalContext

Phase 1 就定义通用运行主体：

```text
PrincipalContext {
  deploymentId
  userId
  deviceId?
  actorType   // USER | AGENT | SYSTEM
  actorId
}
```

S0 Android 请求以 USER 为业务主体并携带 device context；S2 AgentRun 以 AGENT 主体进入授权/Usage，而不要求重做 identity 模型。

### 5.3 外部身份演进

Phase 2 OIDC/SSO/SAML 只负责把外部 subject 映射到稳定 `EnterpriseUser`。Agent Space ownership、Usage、Run、Policy Assignment 仍引用 `usr_*`，不因认证提供方改变而迁移。

### 5.4 Phase 1 Stable ID 基线

Phase 1 不等待 S2/S3 才统一身份。所有新平台实体从首次落地起即遵守平台术语/ID 契约：

```text
I1 Identity
  dep_*  Deployment
  usr_*  EnterpriseUser
  dev_*  Device
  enr_*  Enrollment
  ses_*  Session
  ins_*  Android installation（非授权身份）

I2 Managed Definition
  prv_* / mdl_* / tts_* / asr_* / mcp_*
  pol_* / drf_* / rel_*
  ups_* / sec_* / rte_*

I3/I4 Runtime
  act_* / req_* / int_*

I5 Metering
  usg_* / prc_*

S1 Agent Space     spc_*
S2 Remote Agent   agt_* / run_* / art_*
S3 Runtime Hook   hok_* / hki_*
```

`resourceId` 是 Runtime 协议字段角色，实际值为 `mdl_* / tts_* / asr_* / mcp_*` 等具体资源 ID，不另造 `res_*` namespace。

所有新 MEASIX stable ID 使用 `<prefix>_<UUIDv4>`；Entity ID 永不因 Rename、Provider migration、Secret rotation、Republish、组织变化而重写。Phase 2 Organization/Group 和 Phase 3 Tenant 只增加新的上位标识，不重编号 Phase 1 数据。

## 6. Managed Capability 与 Snapshot

### 6.1 Managed Capability 不是 Android Settings JSON

Control Hub 与 Android 使用独立 Wire Model，不把 Android 当前 `Settings` 持久化结构当作企业协议。

概念 Snapshot：

```text
ManagedCapabilitySnapshot
├── providers / models
├── tts
├── asr
├── mcpServers
├── toolGateway           # S0.3 fixed discover/invoke surface identity/hash
├── search                 # Phase 1 后续可加入
├── managedAssistants       # S0.2 Assistant + mature memory seed
├── skills                  # Phase 1 future，按需渐进披露
├── starters                # S0.2 助手常用入口提示词
├── remoteAgents           # S2
├── runtimeHooks           # S3
└── policy
    ├── agentSpacePolicy   # S1
    ├── featurePolicy
    └── defaults
```

S0.1 首批服务端范围为 Model、TTS、ASR、Direct MCP；S0.2 增加 Managed Assistant/Memory Seed/Assistant Starter 和独立 Enterprise Update Feed；S0.3 增加 Gateway logical resource/surfaceHash 并冻结服务端 Catalog/discover/invoke；S0.4 完成 Android 全 required runtime profile。

Actual `AgentSpace`、`AgentRun` 和 Hook Invocation 不进入 Snapshot。

### 6.2 Realm-aware Local + Managed

```text
EffectiveRuntime(PERSONAL)
  = Built-ins + Personal Local

EffectiveRuntime(ENTERPRISE, deploymentId)
  = Applied Managed Snapshot(deploymentId)
  + policy-allowed User Configuration + Enterprise Preferences
```

Enterprise Realm 的用户及可选 Built-in 配置按类别执行生命周期架构 §4.1 的准入规则；五项之外的明确允许项仍可使用。不可移除的宿主安全/导航能力不属于可选配置，Built-in 资源不能绕过对应类别的策略。

Managed 与用户定义按来源及稳定 ID 区分，不以同 ID 覆盖。用户定义只保存一份，企业域按 Policy 引用原定义和用户凭据，解绑不破坏用户配置。Personal Realm 不把 Managed 内容投影到 picker/tool registry/runtime resolver。

配置三类逻辑归属、五项准入策略和派生生效快照遵循 Enterprise Experience 生命周期架构 §4。定义复用不共享运行数据；聊天、记忆和文件按域与主体隔离。不再建立 Enterprise Local 资源副本；按企业保存的选择和允许调整开关属于用户偏好。

Capability Definition 的只读属性必须在持久化/命令边界强制，而不只隐藏 UI Edit 按钮。

### 6.3 发布与原子性

- Draft 可编辑；
- Release 不可变；
- `managedGeneration` 单调增加；
- Rollback = Republish 历史内容形成新 generation；
- Snapshot 必须 schema/reference/hash/credential reference 完整验证后原子 commit；
- 失败保留 Last Known Good。

S0 采用 **Publish = activate + enforce**；灰度/宽限窗口留 Phase 2。

## 7. Runtime Control 与版本模型

固定四个不同语义：

```text
managedGeneration   客户端 Release/Snapshot 单调版本
controlRevision     Control Hub → Runtime Relay 的完整运行控制版本
gatewayControlRevision Control Hub → Enterprise Tool Gateway 的完整运行控制版本
managedStateRevision 客户端 control state 变化序号
```

S0 不引入 `minimumAcceptedManagedGeneration`。Relay 只接受 `managedGeneration == activeManagedGeneration`；灰度/宽限窗口到 Phase 2 再以新增 rollout policy 扩展。

`controlRevision` 可因 Secret rotation、Upstream operational change、User/Device revoke、auth key rotation 等独立增长，不要求 Android 重下 Snapshot。

### 7.1 Publish 激活：受影响 target 的单一 Desired-State Apply

S0 的单 Hub + 单 Gateway + 单 Relay reference deployment 不引入通用分布式事务协调器。目标 Release 含 Gateway 时，即使 surface/catalog bytes 不变，也必须先建立新 generation 映射，顺序固定为 Gateway-first、Relay-second：

```text
1. Hub validate + create STAGED Release/Snapshot
2. Hub persist Activation + runtimeStatus=ACTIVATING
3. Hub compile GatewayControlState revision G（目标 Release 含 Gateway 时）
4. Gateway validate fully + atomic swap + ACK(G, hash)
5. Hub compile RuntimeControlState revision R
6. Relay validate fully + atomic swap + ACK(R, hash)
7. Hub finalize Release ACTIVE + runtimeStatus=READY
```

安全边界：

- Admin 只有最后 finalize 后才看到 Publish success；
- Gateway apply 失败时 Relay 不切换；Gateway 已 apply 而 Relay 失败时，新 Gateway state 保持 public runtime 不可达的 dormant state；
- Relay apply 是全量状态原子替换，验证失败保持旧状态；
- Hub 在 `ACTIVATING` 时阻断正常客户端新 interaction；
- Relay apply 后 Hub finalize 前的短暂窗口，旧 generation 请求会得到 428，属于安全 fail-closed；
- Hub crash/网络超时通过 Relay `status` 比对 `controlRevision + bundleHash` 后 finalize 或重放 desired state；
- operational change 复用同一 apply/status 协议，不另造第二套 control API。

移除 Gateway 时先让 Relay 移除 route 并 ACK，再清理旧 Gateway state；cleanup 不阻塞安全移除后的 finalize。当前和目标都无 Gateway 才完全跳过 Gateway。Gateway/Relay 都不维护 Prepared/Barrier/Activation authority；Hub 以持久化 Activation intent 和所需 target 的 exact revision/hash status 完成恢复。详细切换协议以 S0 Control Protocol 和 Enterprise Tool Gateway Contract 为权威。

## 8. Runtime Relay 架构边界

### 8.1 请求路径

```text
Transport
 → requestId
 → Session/Auth
 → Principal
 → managedGeneration barrier
 → resourceId authorization
 → resourceId → RuntimeRoute
 → Header/Credential Policy
 → Upstream Transport
 → Stream/Cancellation
 → RequestUsageEvent
```

在 auth/generation/resource/route 检查完成前不得向 Upstream 写 body。

### 8.2 客户端只引用 Resource，不引用 Route

Runtime API 固定为：

```text
/runtime/v1/resources/{resourceId}/{upstreamPath...}
```

`resourceId` 是 `mdl_* / tts_* / asr_* / mcp_* / twg_* ...`。Relay 在 control state 内部解析 `resourceId → runtimeRouteId → Upstream 或 private Gateway target`；Gateway 不是一个伪造的 Upstream。

**`runtimeRouteId` 不进入 Android Snapshot，也不再需要 `X-Measix-Resource-Id`。**这避免客户端同时携带两套执行身份。

### 8.3 Transport

S0 必须支持：普通 HTTP、HTTP streaming/SSE passthrough、TTS binary、ASR multipart/binary、cancellation。

S0 **不实现 WebSocket tunnel**；出现明确 realtime Provider/client 需求后再增加，不保留无实现价值的 skeleton。

Relay 不解析 Provider-specific SSE body，也不自行推算 semantic usage。

## 9. Upstream Adapter 与语义用量

Upstream Adapter 承担：

```text
client protocol compatibility
provider auth / routing
provider-specific translation
provider-specific usage extraction（若具备）
```

平台对 Adapter 只要求稳定 Endpoint、transport/auth/path contract、健康检查、correlation 和明确 Usage Capability。

语义计量优先通过：

```text
X-Measix-Request-Id
virtual key
request log id
usage API
webhook
```

等方式与 平台 `requestId` 关联。

无法获得可靠 semantic usage 时，只记录 request-level 事实并将 semantic usage 标记 UNKNOWN/PARTIAL；禁止 Relay 自己猜 token/音频秒数。

## 10. Agent Space（S1）

### 10.1 稳定语义

Agent Space 是 **Hosted Runtime Resource / 托管执行资源**，归属于 EnterpriseUser/Principal。

```text
AgentSpacePolicy          # Managed Definition/Policy
        ↓
AgentSpace               # Runtime Instance
```

概念实例：

```text
AgentSpace {
  id
  ownerUserId
  diskId
  runtimeId
  status
  storageQuota
  computeProfile
  createdAt
  lastActiveAt
}
```

### 10.2 隔离边界

每个用户 space 使用独立 disk/runtime，由虚拟化/运行引擎执行真实 quota 和隔离；不能用共享目录树模拟独立空间。

WebDAV/MCP 是 access surface：

```text
https://measix.company.com/<workspace-access-path>/...
```

它们不改变底层 disk ownership 和生命周期。

### 10.3 生命周期

Phase 1 采用：

```text
NONE → PROVISIONING → READY → SUSPENDED → DELETED
```

优先 lazy provision：用户首次需要 Hosted Runtime 时创建，而不是 Enrollment 时为每个用户立即启动 VM/runtime。

### 10.4 Metering

至少记录：storage bytes、compute duration；有意义时记录 egress bytes。基础 quota 可约束 storage、compute profile、idle policy。

## 11. Agent Runtime 与 Remote Delegation（S2）

### 11.1 AgentDefinition 与 AgentRun

```text
AgentDefinition {
  agentId
  name
  instructions
  modelRef
  toolPolicy
  agentSpacePolicy?
  enabled
}

AgentRun {
  runId
  agentId
  ownerUserId
  agentSpaceId?
  callerInteractionId?
  status
  startedAt
  finishedAt
  usage
}
```

`Remote Agent` 是 AgentDefinition 被暴露为 delegation target 的角色，不创建第二套 Definition 类型或 `remoteAgentId`。

### 11.2 Agent Runtime 边界

Agent Runtime 负责 Run create/status/cancel/timeout、execution lifecycle、artifact/event/usage。Phase 1 不负责 durable scheduler、持续驻留 Agent、checkpoint/resume 或 Fleet orchestration。

S2 初版允许作为 Agent Space 服务内的独立模块运行，以减少额外 daemon；必须通过清晰内部接口保持未来可拆分性，但不提前引入消息队列。

### 11.3 Android Delegation

```text
LOCAL_ASSISTANT | REMOTE_AGENT
```

Master 显式提交 request/context/attachments，等待 request-scoped AgentRun 结果后继续本地流程；不上传全部 Conversation。

### 11.4 Phase 2 演进

Phase 2 在相同 AgentDefinition/AgentRun 基础上增加 AgentInstance、AgentFleet、Trigger、checkpoint/resume。不得通过把 S2 的一个 Run 永久循环来模拟 Agent Fleet。

## 12. Runtime Hook / Conversation Intervention（S3）

### 12.1 产品能力与实现原语

产品能力名称：**Conversation Intervention / 会话介入**。  
实现原语：**Runtime Hook / 运行时钩子**。

Hook 是运行控制机制，不是和 Model/MCP 同类的普通资源。

### 12.2 Hook Point

S3 首先实现：

```text
before_generation
before_tool_call
```

架构预留：

```text
before_model_request
after_model_response
after_tool_result
```

禁止任意“执行一段企业脚本修改客户端状态”的无界 Hook。

### 12.3 Result

稳定结果：

```text
ALLOW
DENY
INJECT_CONTEXT
REQUIRE_APPROVAL
```

Hook Definition 具有 timeout 与 failureMode：

```text
FAIL_OPEN
FAIL_CLOSED
```

Context enrichment 一般 FAIL_OPEN；安全/DLP 类策略可以 FAIL_CLOSED。

### 12.4 Interaction 一致性

一次 interaction 启动时捕获其 `managedGeneration + hook configuration`。运行中的 interaction 不因为后台 Publish 而原地改写 Hook 定义；下一 Runtime boundary/新 interaction 按强制 generation 机制切换。

Relay-side hard revoke/authorization 仍可由 Relay operational control 立即生效。

## 13. Metering、Cost 与 Quota

### 13.1 Usage Ledger

核心保持 append-only，概念事件：

```text
UsageEvent {
  id
  deploymentId
  userId
  actorType
  actorId
  resourceType
  resourceId
  operation
  quantity
  unit
  occurredAt
  requestId?
  interactionId?
  runId?
  managedGeneration?
}
```

### 13.2 典型 Meter

Model：

```text
INPUT_TOKENS
OUTPUT_TOKENS
CACHED_TOKENS
REQUESTS
```

TTS/ASR：

```text
CHARACTERS
AUDIO_SECONDS
REQUESTS
```

Remote Agent：

```text
RUN_COUNT
RUN_DURATION_MS
TOOL_CALL_COUNT
```

Agent Space：

```text
COMPUTE_MILLISECONDS
STORAGE_BYTES
EGRESS_BYTES
```

Runtime Hook：

```text
HOOK_INVOCATIONS
HOOK_DURATION_MS
```

Hook 内部如果调用模型，模型用量通过普通 Model usage 记录，不重复计费。

### 13.3 Request Usage 与 Semantic Usage

```text
Relay Request Event
          + requestId
Upstream Semantic Usage
          ↓
Usage Ledger / Cost
```

Relay request event 使用本地 append-only spool + at-least-once ingest，Control Hub 按 `requestId` 去重，避免 Control Hub 短时不可达导致 usage 静默丢失。

### 13.4 Quota

Quota 与 Metering 分离，但 Phase 1 不提前建设通用 Quota Engine。

- S0：只完成 Usage/Cost 事实；
- S1：Agent Space 对 storage/compute profile/idle policy 做组件内真实资源限制；
- S2/S3：run timeout、hook timeout 等属于执行安全边界，不抽象成通用 quota；
- Phase 2：出现 Organization/Fleet/cost-center 等治理需求后，再建立跨资源 quota/policy。

SaaS Billing 不属于 Phase 1。

## 14. Correlation 与运行标识

Phase 1 固定：

```text
managedGeneration  企业客户端配置版本
controlRevision     Relay 运行控制版本
gatewayControlRevision Gateway 运行控制版本
managedStateRevision managed state 变化序号
interactionId       Android 一次顶层交互 int_*
requestId           单个 Runtime 请求 req_*
runId               Remote Agent 等长运行实例 run_*
activationId        一次 Control Hub 发布/Relay apply 操作 correlation
```

关系示例：

```text
User usr_1
  └─ interaction int_1 @ managedGeneration=42
       ├─ request req_1
       ├─ request req_2
       └─ remote run run_1
            ├─ request req_3
            └─ Agent Space usage
```

这套 ID 必须贯穿 Logs、Usage、Audit 和问题排查。

## 15. Android 集成边界

Android 不建立第二套 Enterprise Chat 架构。

Android 建立两个顶层 ClientRealm，但复用同一套 Chat/Assistant/Conversation/Tool Loop 实现：

```text
PERSONAL   → Personal effective view → existing local runtime
ENTERPRISE → ManagedStateGuard → enterprise effective view → existing runtime + Relay
```

```text
Enterprise top-level interaction
  → ManagedStateGuard
  → EffectiveRuntimeSnapshot(ClientRealm, deploymentId, managedGeneration)
  → existing GenerationHandler
  → ManagedRuntimeInterceptor / Local runtime
```

Enterprise 逻辑职责包括 Identity、Realm、Managed State、Experience projection、Portal host/session/bridge、Runtime adaptation 和 metering context。S2 delegation、S3 intervention 只在对应阶段进入；具体 package/class/file 由 Android 实现仓库按真实 owner 决定，不在架构中预建目录树。

关键不变量：

- `SettingsStore` 仍表示 Local Settings；
- Managed Snapshot 存独立 Store；
- Local record 必须保留 Personal/Enterprise realm scope，Managed record 必须保留 deployment ownership；
- Conversation、Assistant、Memory、Attachment 和 Workspace 不通过 Realm 切换隐式改变归属；
- Enterprise credential 不进入普通 Settings/Backup；
- Managed/User 按来源及稳定 ID 独立引用，不覆盖用户原定义；
- Managed ID mutation 在写边界拒绝；
- `GenerationHandler` 不直接依赖 Admin/Control Hub 领域类型。

## 16. 强制配置与离线语义

正确性只依赖两层：

1. Android 每个新企业 interaction 前访问 Control Hub Managed State；
2. Runtime Relay 对每个请求执行 active generation barrier。

S0 不要求前台 SSE、Device heartbeat 或后台 push。它们都是后续 UX/运营优化，不承担 consistency。

完全离线时不能知道后台刚发生的变化，因此企业绑定状态下若 preflight 无法成功，新企业 Runtime fail closed；本地历史和设置仍可浏览。

若运行中的 Tool Loop 下一次 Relay 请求得到 428，当前 interaction 终止；同步新 Snapshot 后由上层创建新 interaction，不在同一 Tool Loop 中静默切 generation。

## 17. Phase 1 S0–S3 实施与验收

### S0 — Foundation Contract

```text
S0.1 Admin → Control Hub → A/B Publish → Runtime Relay
S0.2 Realm → Assistant/Seed/Starter + Enterprise Update Feed/Portal foundation
S0.3 Admin/Test Client → Hub → Enterprise Tool Gateway + Relay → platform/downstream MCP
S0.4 Android preflight/Snapshot v5 → Relay → Upstream Adapter/Gateway → Usage Ledger
```

验收：Personal/Enterprise Realm 隔离，A/B/C 最小闭环与 Portal 企业动态可用，Model/TTS/ASR/Direct MCP/Gateway 真实调用、旧 generation 被阻断、Secret 不下发客户端、Usage/Audit 可归属。

### S1 — Agent Space

验收：独立持久 disk/runtime、MCP/WebDAV access、storage/compute usage、ownership 隔离。

### S2 — Agent Runtime & Remote Delegation

验收：Android 可同步委派到 AgentDefinition；AgentRun 与 User/AgentSpace/interaction 关联；cancel/timeout/artifact/usage 可追踪；不引入 durable Fleet。

### S3 — Runtime Hook

验收：`before_generation` 和 `before_tool_call` 真实执行；timeout/failureMode/audit/usage 可观测；Hook 不直接写 Android persistence。

## 18. Phase 1 安全与故障边界

1. 企业 Shared Secret 只存在 Control Hub / Runtime Relay 安全边界；
2. Relay internal API 不暴露公网；
3. Control Hub 不进入通用 Runtime 大流量 data path；Enterprise Update 仅保留 Gateway private typed read projection；
4. Relay 未通过 Auth/Generation/Authorization/Route 前不写 Upstream body；
5. Hub desired Relay/Gateway control 与各自 applied control 可独立 reconcile；
6. Relay/Gateway restart 且无有效 control state 时 fail closed，等待 Hub rehydrate；
7. Android LKG 不能绕过明确 revoke/新 generation；
8. Usage 可以最终补送，但不能静默丢失或伪造 semantic usage；
9. Phase 1 不是 MDM；
10. Adapter 故障不能污染 Capability Snapshot 真源。

## 19. 向 Phase 2 的演进接口

Phase 2 直接增加而不是推翻：

- Organization/Group/RBAC 建立在稳定 User/Principal 上；
- OIDC/SSO 映射现有 User；
- User Sync 与 Capability Snapshot 独立；
- User Sync 承载企业助手记忆、Experience Contribution 和场景/任务状态，而新版正式经验仍通过 Managed Release/Snapshot 下发；
- Enterprise Portal 在 S0.2 MVP 上扩展企业用户同步、备份、经验库和触达流程，不拥有 Android 本地状态；
- Experience Curation 把记忆/贡献提升为新 ManagedAssistant/Skill/Starter Definition，不原地修改已发布 Release；
- Agent Space 扩展 capacity/admin lifecycle；
- Agent Runtime 增加 AgentInstance/checkpoint/resume；
- Agent Fleet 增加 fleet/trigger/concurrency/operations；
- Enterprise Connector 接 ERP/CRM/DB/SaaS；
- Runtime Hook 扩展更多 Hook Point；
- rollout policy 再引入 accepted generation window；
- Quota/Audit/Retention 增强。

**不提前固定 Phase 2 的微服务拆分**。Agent Runtime/Fleet/Connector 是否拆进程，由实际并发、可靠性和团队边界决定。

## 20. Phase 1 完成后的平台能力

```text
EnterpriseUser / Principal
      │
      ▼
Capability / Policy ───────── 企业允许什么
      │
      ├─ Agent Space ──────── workspace/compute
      ├─ Agent Runtime ────── AgentDefinition → AgentRun
      │      └─ Remote Agent  delegation role
      └─ Runtime Hook ─────── policy/context intervention
             │
             ▼
         Runtime Relay
             │
             ├─ Upstream Adapter
             └─ Usage Ledger
```

Phase 1 S0.2 为 Enterprise Experience 提供 ClientRealm、Managed Assistant/Memory Seed/Starter、Enterprise Update、Portal MVP、Definition/Release/Snapshot 和 provenance 基础。Phase 2 的 Agent Fleet、Enterprise Connector、User Sync、Portal 扩展和经验提炼都直接建立在这些基础身份和运行原语上。
