# MEASIX Platform 术语与标识符长期契约

> 状态：Platform-wide Terminology & Identifier Authority / 平台级术语与标识符权威基线  
> 版本：2026-08-31
> 上位文档：`measix-agent-platform-roadmap.md`  
> 适用范围：Phase 1 / Phase 2 / Phase 3 及所有 MEASIX 自有组件  
> 文档职责：固定 MEASIX 长期使用的核心概念、对象身份、字段命名、ID namespace、生成/稳定性规则以及版本号与标识符的区别。下位架构、协议、代码和数据库不得自行创造与本文冲突的同义词或 ID 语义。

---

## 1. 为什么需要这份契约

MEASIX 会经历从单企业自托管 Runtime Foundation，到完整 Enterprise Agent Platform，再到 Multi-tenant Agent Platform 的长期演进。真正危险的不是某个字段名称不够漂亮，而是同一个词在不同阶段代表不同对象，或同一个对象在后续阶段被迫换 ID。

因此平台从 S0 开始固定两类长期契约：

1. **Terminology Contract**：一个名词只代表一个稳定概念；
2. **Identifier Contract**：一个对象一旦获得 MEASIX ID，该 ID 在对象生命周期内不可变、不可复用、不可因名称/组织/Provider/部署模式变化而重写。

本文优先解决以下长期问题：

```text
“企业”到底由什么标识？
“用户”换邮箱/SSO 后是不是同一个人？
Android 安装 ID 和服务端 Device ID 是否等价？
Model 的 MEASIX ID 和 Provider model name 是否等价？
Managed Resource、Runtime Route、Upstream 是否可以共用一个 ID？
managedGeneration / controlRevision 是不是 ID？
Phase 3 引入 Tenant 后 Phase 1 的 deploymentId 怎么办？
Agent Definition 和一次 Run 是否是同一个对象？
```

确定答案后，后续代码必须沿用，不因实现语言或数据库变化重新定义。

---

## 2. 文档权威关系

平台文档层级调整为：

```text
measix-agent-platform-roadmap.md
  ├─ measix-platform-terminology-and-identifier-contract.md   # 本文：跨 Phase 长期术语/ID 权威
  └─ measix-runtime-foundation-architecture.md
       └─ measix-s0-foundation-contract-spec.md
            ├─ measix-s0-implementation-decision.md
            ├─ measix-s0-control-protocol.md
            ├─ measix-s0-control-hub.md
            ├─ measix-s0-runtime-relay.md
            ├─ measix-s0-enterprise-tool-gateway-contract-spec.md
            ├─ measix-s0-enterprise-tool-gateway.md
            ├─ measix-s0-android-integration-contract-spec.md
            ├─ measix-s0-admin-console-product-requirements.md
            └─ measix-s0-upstream-adapter-contract.md
```

冲突处理：

- 平台概念名称、对象身份、ID 字段名称、ID namespace 与长期稳定规则，以本文为准；
- Phase 1 组件边界以 Runtime Foundation 架构为准；
- S0 wire/state 行为以 Control Protocol 为准；
- 实现顺序/语言/ORM/第三方包以 Implementation Decision 为准；
- 单组件只决定“如何实现本文中的对象”，不得重新给对象命名或重新分配 ID 语义。

---

### 2.1 品牌、子系统与技术角色命名规则

`MEASIX` 是平台品牌，不是所有组件的前缀。长期文档和代码必须区分：

```text
Brand / Product       MEASIX Agent Platform
Phase name            Runtime Foundation / Enterprise Platform / Multi-tenant Platform
Subsystem name        Control Hub / Runtime Relay / Agent Space / Agent Runtime / Agent Fleet ...
Technical role        HTTP server / reverse proxy / worker / scheduler / database
Domain entity         User / Model / Agent / Run / Release / Route ...
```

组件正式名称固定为：

| 名称 | 类别 | 稳定语义 |
|---|---|---|
| Android Client | Experience | 本地 Runtime 与企业能力消费端 |
| Enterprise Portal | Experience | 企业用户工作台；负责同步、备份、经验库、通知/场景等用户流程，不拥有 Android 本地状态 |
| Admin Console | Experience | 企业管理 UI，只调用 Control Hub |
| Control Hub | Control Plane | Identity、Capability Catalog、Policy/Release、Runtime Desired State、Usage 的权威中心 |
| Runtime Relay | Runtime Plane | Runtime admission、授权、Route/Credential、透明 L7 relay、request metering |
| Enterprise Tool Gateway | Runtime Plane | Enterprise Realm 固定 Meta Tool surface、Published Tool Catalog discovery、toolRef/schema enforcement 与受治理代理执行 |
| Agent Space | Runtime Plane | 用户级持久、隔离 workspace/runtime resource |
| Agent Runtime | Runtime Plane | Agent Definition/Run/Instance 的执行生命周期 |
| Remote Agent | Agent role | Agent Definition 被暴露为 delegation target 的角色；不是独立服务类型 |
| Agent Fleet | Agent operations | 企业持续运行、可恢复、可调度的 Agent Instance 集群 |
| Runtime Hook | Governance | 固定运行点的上下文/策略/审批介入机制 |
| Enterprise Connector | Integration | ERP/CRM/DB/SaaS 等企业业务数据与动作接入 |
| Upstream Adapter | Integration | AI/Speech/MCP Provider 协议兼容边界 |
| User Sync | User Data | Phase 2 用户数据增量同步子系统 |
| Enterprise Experience | Domain | 企业助手、Skill、记忆、场景和经验贡献的提炼与再发布领域；不是单一微服务 |
| Usage Ledger | Metering | append-only 用量事实 |

规则：

1. 正常叙述写 `Control Hub`，不写 `MEASIX Control Server`；
2. 正常叙述写 `Runtime Relay`，不写 `MEASIX Runtime Gateway`；
3. 泛称 `server`、`gateway`、`proxy`、`worker` 只描述实现角色；`Enterprise Tool Gateway` 是完整注册专名，不与 Runtime Relay 混称；
4. 文档文件名中的 `measix-` 仅是文档命名空间，不代表组件正式名称；
5. Phase 2/3 只有在职责已经稳定时才注册新的子系统名称，不为了架构图完整而预造微服务。

`Relay` 用于强调“在客户端和目标之间执行受控 L7 转发”；`Hub` 用于强调控制状态的集中权威。两者是职责名，不承诺具体网络拓扑。

## 3. 命名的四个层级

MEASIX 中必须区分四种概念，避免一个“资源/ID”承担多个角色。

### 3.1 Entity / Stable Object — 稳定实体

具有独立生命周期、需要被其他对象长期引用的对象，例如：

```text
Deployment
EnterpriseUser
Device
ManagedModel
Upstream
ManagedRelease
AgentSpace
AgentDefinition
```

这类对象必须有长期稳定 Entity ID。

### 3.2 Definition / Policy — 定义与策略

描述“允许什么/如何配置”，但不是一次实际执行：

```text
ManagedProviderDefinition
ManagedModel
ManagedPolicy
AgentDefinition
RuntimeHookDefinition
AgentSpacePolicy
```

Definition 可以形成 Release/Snapshot；其 stable ID 不因为新 Release 而改变。

### 3.3 Runtime Instance / Execution — 运行实例

一次真实执行或运行资源：

```text
EnterpriseSession
RuntimeInteraction
RuntimeRequest
AgentSpace
AgentRun
RuntimeHookInvocation
```

其中长生命周期实例使用 Entity/Run ID；请求链使用 Correlation ID。

### 3.4 Revision / Generation / Hash — 版本与摘要

以下不是实体 ID：

```text
managedGeneration
controlRevision
gatewayControlRevision
managedStateRevision
draftRevision
schemaVersion
snapshotHash
bundleHash
```

它们描述某一 scope 内的顺序、版本或内容，不得被当成对象主键替代 stable ID。

---

## 4. Identity / Governance 术语

### 4.1 Tenant — 租户（Phase 3）

**定义**：多租户平台中的最高商业、安全、数据隔离和计费边界。

```text
tenantId = tnt_<uuid>
```

Phase 1/S0 不实现 Tenant，也不为了未来提前给所有 API 添加假 tenant 参数。

长期关系：

```text
Tenant
  └─ one or more Deployment
```

**关键不变量**：Phase 3 引入 `tenantId` 时，既有 `deploymentId` 不改名、不重写、不复用为 tenantId。

### 4.2 Deployment — 部署实例 / 控制域

**定义**：一套具体 MEASIX 控制与运行配置域，是 Release、managedGeneration、Control State 的作用范围。

```text
deploymentId = dep_<uuid>
```

Phase 1：一个 Deployment 约等于一个企业自托管实例，因此它就是当前“企业”的技术标识。  
Phase 3：Deployment 成为 Tenant 下的具体部署/区域/环境，不失去原身份。

因此长期代码中禁止新增语义模糊的：

```text
enterpriseId
companyId
customerId
```

S0 需要表达当前企业范围时使用 `deploymentId`。

### 4.3 Organization — 组织（Phase 2）

**定义**：Tenant/Deployment 内用于治理的组织单元，不是部署边界。

```text
organizationId = org_<uuid>
```

Organization 未来承载部门/业务组织层级；它不能替代 Tenant 或 Deployment。

### 4.4 Group — 组（Phase 2）

**定义**：用于 Capability/Policy Assignment 的成员集合。

```text
groupId = grp_<uuid>
```

### 4.5 Membership — 成员关系（Phase 2）

**定义**：User 与 Organization/Group 之间有生命周期的成员关系。若成员关系具有角色、有效期、来源等独立状态，必须建成实体而不是简单数组。

```text
membershipId = mbr_<uuid>
```

### 4.6 EnterpriseUser — 企业稳定用户

**定义**：MEASIX 内部长期稳定的人类用户身份。

```text
userId = usr_<uuid>
```

以下全部只是属性或外部映射，**不是 User ID**：

```text
email
username
phone
OIDC subject
SAML NameID
SCIM externalId
employee number
```

用户改名、换邮箱、切换身份提供方、进入不同 Group 都不能改变 `userId`。

#### Administrator 不是独立身份类型

`Administrator` 是 EnterpriseUser/Principal 获得的控制面角色或权限，不创建长期 `adminId` namespace。

S0 可以只有：

```text
ADMIN | MEMBER
```

Phase 2 再演进为 RBAC。审计字段使用 `actorUserId` / `actorPrincipal`，不使用 `createdByAdminId` 作为长期模型。

### 4.7 Device — 服务端注册设备身份

**定义**：一个 EnterpriseUser 注册到 MEASIX 的客户端安装实例，是服务端授权/revoke/rollout 的稳定对象。

```text
deviceId = dev_<uuid>
```

由 Control Hub Enrollment 成功时生成。

Device 与 Android 自己生成的 installationId 不同：

```text
installationId = ins_<uuid>   # Android local installation correlation
```

`installationId` 只能用于重复安装检测/诊断/Enrollment 元数据，不能成为授权身份。Relay 和业务权限只相信 Control Hub-issued `deviceId`。

### 4.8 Enrollment — 一次性绑定凭据记录

**定义**：管理员授权某个 EnterpriseUser 建立新 Device 的一次性、短时记录。

```text
enrollmentId = enr_<uuid>
```

Enrollment Code 是 Secret Credential，不是 ID。数据库保存：

```text
enrollmentId
tokenHash
userId
expiresAt
consumedAt
```

UI 可显示一次 code，但日志/URL 长期记录不得以 code 代替 enrollmentId。

### 4.9 Session — 认证会话

**定义**：Principal 当前认证生命周期的服务端对象。

```text
sessionId = ses_<uuid>
```

Android access token / refresh token、Admin Web cookie 都是认证凭据；凭据可以轮换，`sessionId` 才是会话身份。

Phase 1 可以按 `channel = ANDROID | ADMIN_WEB` 区分会话。Service-to-service credential 不冒充 User Session。

### 4.10 Principal — 当前执行主体

**定义**：一次请求/执行“代表谁”的统一运行身份，不要求 Principal 本身拥有一个独立数据库主键。

```text
PrincipalContext {
  deploymentId
  userId?
  deviceId?
  actorType   // USER | AGENT | SYSTEM
  actorId
}
```

`actorId` 指向当前 actor 的稳定 ID，例如 `usr_*`、`agt_*` 或 `agi_*`。Principal 是解析结果，不另造 `principalId`，除非未来出现需要独立生命周期的 Principal 对象。

### 4.11 ClientRealm — Android 产品、数据与执行域

**定义**：Android 当前用户体验、数据可见性和 Effective Runtime 的顶层作用域。

```text
ClientRealm = PERSONAL | ENTERPRISE
```

- `PERSONAL` 不携带 `deploymentId`，使用 Built-in 与用户配置；
- `ENTERPRISE` 必须携带 `deploymentId`，使用该 Deployment 的 Managed 内容与 Policy 允许直接引用的用户配置；
- `BOUND` 表示已建立 Enterprise Binding，不等于当前 `ClientRealm=ENTERPRISE`；
- Conversation、运行 Memory、Attachment、Workspace 和运行产物必须具有可确定的域与主体归属，不得通过 UI 切换隐式改变归属。用户 Assistant 等配置定义只有一份，其运行数据另按域与主体保存。

`origin` 与 `realm` 是两个正交维度：

```text
origin = BUILT_IN | LOCAL | MANAGED
realm  = PERSONAL | ENTERPRISE(deploymentId)
```

`origin` 描述定义来源，`realm` 描述使用域，不能把二者当成资源必须复制的存储分区。`MANAGED` 定义归属其 Deployment；`LOCAL` 用户定义只保存一份，可按 Policy 在企业域使用。资源引用必须保留来源与稳定 ID，Managed/User 同 ID 不构成覆盖关系。运行记录另外保存其创建时的域与主体。

**User Configuration / 用户配置**：用户 A/B/C 定义、个人资料及个人备份配置。**User Preferences / 用户偏好**：公用显示/操作偏好及各域分别保存的选择、允许调整的使用参数和开关。**Enterprise Delivered Configuration / 企业下发配置**：按企业保存、随快照整体更新的受管定义及 Policy。以上是逻辑归属，不规定物理存储数量。

**Effective Configuration Snapshot / 生效配置快照**：按当前域、主体、适用企业版本与 Policy 派生的只读模型，表达资源、助手、工具、实际选择与参数，以及来源、可选择、可修改、强制启用和不可用原因。它不构成额外持久化真源。

---

## 5. Managed Capability / Control Plane 术语

### 5.1 Managed Capability — 受管能力集合

**定义**：企业发布给用户的 Definition/Policy 总体概念，不是数据库中的单一“capabilityId”。

包含：Provider/Model、TTS、ASR、MCP、Policy，以及后续 Agent Definition、Runtime Hook Definition、Agent Space Policy 等。

`Managed Capability` 是平台技术交付总称，不等于产品分类中狭义的“B 能力”。企业交付在产品上固定分为：

```text
A  Runtime Resource       Model / TTS / ASR / Image / compute / storage
B  Enterprise Capability MCP / Search / Connector / Remote Agent / business action
C  Experience Asset      Managed Assistant / Skill / memory seed / workflow / starter
```

ManagedPolicy 是横切 A/B/C 的治理层，不是第四种业务内容。同一个底层实现可以在不同语义角色中被引用，但 Definition 必须有唯一 owner 和 stable ID。

禁止创建一个无语义的全局 `capabilityId` 来替代各具体对象身份。

### 5.2 ManagedProviderDefinition — 客户端 Provider 定义

**定义**：Android 可见、用于组织一个或多个 Model 和 client protocol 的逻辑 Provider 定义。

```text
providerId = prv_<uuid>
```

它是客户端定义，不等同于服务端 Upstream。

### 5.3 ManagedModel — 受管模型

**定义**：企业向客户端暴露的一个逻辑 AI Model 能力。

```text
modelId = mdl_<uuid>
```

模型的 MEASIX 身份与 Provider 的真实 model key 必须分离：

```text
modelId           = mdl_...            # 长期稳定 MEASIX identity
upstreamModelKey  = "gpt-..."          # Provider/Adapter-owned mutable selector
providerId        = prv_...             # client-visible provider definition
runtimeRouteId    = rte_...             # server-side execution route
```

`upstreamModelKey` 可以因 Adapter/Provider 迁移而改变；只要企业逻辑 Model 仍是同一资源，`modelId` 不变。

### 5.4 Managed TTS / ASR / MCP

S0 直接固定：

```text
ttsId = tts_<uuid>
asrId = asr_<uuid>
mcpServerId = mcp_<uuid>
```

未来同类资源继续使用各自 namespace，例如：

```text
searchId    = srch_<uuid>
imageId     = img_<uuid>
embeddingId = emb_<uuid>
videoId     = vid_<uuid>
```

#### resourceId 的稳定语义

Wire/Runtime 中的 `resourceId` 是**协议角色字段**，不是新的 `res_*` namespace：

```text
resourceId = one of:
  mdl_*
  tts_*
  asr_*
  mcp_*
  twg_*
  ...future managed resource IDs
```

Runtime URL 的 `{resourceId}` 携带具体资源 ID，例如 `/runtime/v1/resources/{resourceId}/...`。Resource kind 一经创建不可原地改变；要从 Model 变为其他 kind 必须创建新资源 ID。

### 5.5 ToolGatewayDefinition — 客户端 Gateway 逻辑资源

```text
toolGatewayId = twg_<uuid>
```

`ToolGatewayDefinition` 表示一个 Deployment 发布给 Enterprise Realm 的 Gateway 逻辑 MCP resource。它进入 Snapshot v5，只携带 client protocol/runtime path/surface version/hash 与 `clientEnablementPolicy` 等客户端安全内容，不携带 Tool Catalog、Tool Integration、downstream endpoint、credential 或 toolRef。

`toolGatewayId` 可以作为 `/runtime/v1/resources/{resourceId}/...` 的 `resourceId`；它不表示 Gateway daemon 实例，也不等于某个 downstream MCP Server。

### 5.6 ToolGatewayProfile — Deployment Gateway 发布配置

S0.3 每个 Deployment 最多一个 active `ToolGatewayProfile`，因此不创建独立 profile ID。它保存 enabled、`REQUIRED | USER_CONTROLLABLE_DEFAULT_ON` client policy、enterprise discover/invoke guidance 和 highlighted `gatewayToolId` references，随 Managed Release 版本化。Profile disabled 时 compiler 从 Snapshot 省略整个 `ToolGatewayDefinition`，wire 不再携带平行 enabled bit。

Profile 不拥有平台固定工具名/安全合同；企业可编辑 segment 不能覆盖 `discover_tools` / `invoke_tool` 的平台不变量。两个工具是 Enterprise Realm 内置、默认开启且不可拆分的原子 pair；client policy 只决定用户能否成对关闭。Direct Managed MCP 仍由 Assistant 引用，不与该 pair 共用 enablement。

### 5.7 ToolIntegration / GatewayToolDefinition — Gateway 工具来源与发布工具

```text
toolIntegrationId = tin_<uuid>
gatewayToolId     = gtl_<uuid>
```

- `ToolIntegration` 是 Gateway-only 的 server-side downstream MCP source/credential/config identity，不进入 Android Snapshot；
- `GatewayToolDefinition` 是从某 ToolIntegration 或 platform adapter 审核发布的稳定工具身份；source name、agent-facing name、description/schema 可以随新 Release 改变，`gatewayToolId` 不变；
- Direct Managed MCP 继续使用 `McpDefinition (mcp_*)`。`ToolIntegration` 与 `McpDefinition` 不是一个带可变 mode 的双重实体，也不存在运行时 fallback。

### 5.8 toolRef — 短期调用能力引用

`toolRef` 是 Enterprise Tool Gateway 在 discover 中签发、绑定 deployment/user/device/session/interaction/generation/tool/schema/expiry 的 opaque capability reference。它不是 stable ID、不得持久作为业务主键、不得由客户端构造，也没有 `trf_*` namespace。

### 5.9 ManagedAssistantDefinition — Android 企业助手定义

```text
assistantDefinitionId = asd_<uuid>
```

定义企业下发、由 Android 本地 Runtime 执行的 Assistant，可引用 Model、Enterprise Capability、Skill、初始经验和 Memory Policy。它与 S2 `AgentDefinition (agt_*)` 不是同一身份：前者是 Android 会话助手定义，后者由 Agent Runtime 执行并可作为 Remote Agent 委派目标。

### 5.10 ManagedSkillDefinition — 企业 Skill 定义

```text
skillId = skl_<uuid>
```

Skill 是可发现、可版本化、可被 Assistant/Agent 引用的经验定义，包含 metadata、主指令和按需加载的 reference/script/asset。Skill 的使用权限、兼容性、依赖和执行边界由 Release 引用的 Policy 约束。

### 5.11 AssistantStarterDefinition — 企业助手常用入口

```text
starterId = str_<uuid>
```

S0.2 的 AssistantStarterDefinition 只组合一个 `ManagedAssistantDefinition`、入口标题和预填提示词。它是可被 Managed Release 发布、由企业动态维护的 Experience Asset；点击只预填 Draft Conversation，不 auto-send、不自动调用工具。未来表单、附件、任务或 workflow 起点必须定义独立类型，不扩张本对象语义。向某个用户定向投递 Starter 则是 EnterpriseActivity，不是 Definition 更新。

### 5.12 ManagedPolicy — 受管策略

```text
policyId = pol_<uuid>
```

Policy 是可被 Release 引用的稳定 Definition。内容变化不改变 policyId；历史值由 Release/Snapshot 保存。

### 5.13 ManagedDraft — 编辑工作区

```text
draftId = drf_<uuid>
draftRevision = monotonic integer
```

S0 即使只有一个 active Draft，也保留 stable draftId，避免 Phase 2 多 Draft/多 scope 时重做 API。

`draftRevision` 只做 optimistic concurrency，不是 Draft identity。

### 5.14 ManagedRelease — 不可变发布版本

```text
releaseId = rel_<uuid>
managedGeneration = monotonic integer scoped to deploymentId
```

Release identity 与 generation 分开：

- `releaseId`：某一次不可变 Release 实体；
- `managedGeneration`：该 Deployment 下客户端版本顺序。

Republish 历史内容：创建新的 `releaseId` 和新的 generation，旧 releaseId 永不复用。

### 5.15 Managed Snapshot — 客户端版本化快照

Snapshot S0 不额外创建 `snapshotId`。稳定定位使用：

```text
(deploymentId, managedGeneration)
releaseId
snapshotHash
```

避免同时存在 `snapshotId + releaseId + generation` 三个都代表同一版本的标识。

### 5.16 Managed State — 当前客户端控制状态

Managed State 是 Deployment singleton 状态，不需要 `managedStateId`。

```text
activeReleaseId
activeManagedGeneration
managedStateRevision
```

### 5.17 Upstream — 服务端上游运行端点

```text
upstreamId = ups_<uuid>
```

Upstream 表示 CLIProxyAPI/LiteLLM/New API/Speech Gateway/企业内部服务等实际 runtime endpoint 定义。

它与 `providerId`、`modelId` 都不同：

```text
providerId   → 客户端配置/协议语义
modelId      → 用户看到并使用的逻辑模型
upstreamId   → Control Hub/Runtime Relay 管理和连接的后端
```

Upstream base URL、health、secret 可以改变而不改变 upstreamId。

### 5.18 ManagedSecret — 企业 Secret

```text
secretId = sec_<uuid>
```

`secretRef` 的值就是 `sec_*`。Secret value/密文/keyVersion 可以轮换，secretId 保持稳定；如果安全策略要求完全新建凭据，也可以创建新 secretId 并切换引用。

Secret ID 可以出现在 Hub/Relay control，但不能把 secret value 放入 Android Snapshot。

### 5.19 RuntimeRoute — Relay 内部执行路由

```text
runtimeRouteId = rte_<uuid>
```

RuntimeRoute 是 Control Hub 编译并下发给 Runtime Relay 的内部执行绑定：`resourceId → typed target + path/method/credential policy`。S0.1 target 是 Upstream；S0.3 增加独立 private Enterprise Tool Gateway target，不把 Gateway 伪装为 `ups_*`。

S0 中它作为 `ManagedDraft/ManagedRelease` typed aggregate child 持久化，不要求独立数据库表；是否在后续阶段正规化为独立表不改变 `runtimeRouteId` 的长期身份。

**长期不变量**：

- `runtimeRouteId` 不进入 Android Snapshot，不作为客户端 Runtime URL 的输入；
- Android/其他调用方只引用逻辑 `resourceId`；
- Runtime Relay 使用 `resourceId` 在当前 control state 中解析 route；
- Upstream/Route 迁移不改变客户端 Model/TTS/ASR/MCP/Tool Gateway stable ID。

这样客户端不会同时携带 `resourceId + runtimeRouteId` 两套执行身份。

### 5.20 PricingRule — 价格规则

```text
pricingRuleId = prc_<uuid>
```

价格规则具有有效期和 meter scope；价格变更新建/版本化规则，不修改 Usage 事实。

---

## 6. Runtime Plane 术语

### 6.1 RuntimeInteraction — 顶层运行交互

Android/客户端发起的一次顶层 Chat/Agent/TTS/ASR/MCP 运行上下文：

```text
interactionId = int_<uuid>
```

由发起客户端生成。一次 interaction 固定一个 `managedGeneration + EffectiveRuntimeSnapshot`，可以包含多个 RuntimeRequest，并可创建 AgentRun。

### 6.2 RuntimeRequest — 单次 Runtime Relay 请求

```text
requestId = req_<uuid>
```

由 Runtime Relay 在请求进入时生成，是日志、Usage、Upstream correlation 的单请求权威 ID；客户端不能指定。

### 6.3 Activation — 一次 Release 发布激活操作

```text
activationId = act_<uuid>
```

由 Control Hub 在 Publish 开始时生成，用于 Admin 长操作恢复、幂等结果和故障诊断。它是 Control Hub 的操作实体，不要求 Runtime Relay 保存独立 Activation 状态机。

S0 的 Relay 一致性依赖 `controlRevision + bundleHash`；Gateway 一致性依赖 `gatewayControlRevision + bundleHash`。Hub 持久化 activation intent；目标 Release 含 Gateway 时先 apply/ACK Gateway 的新 generation 映射，再 apply/ACK Relay、finalize。Gateway 移除和 cleanup 由 Control Protocol 的限制性切换规则定义，不能只按 Gateway 内容是否变化判断参与。

### 6.4 Idempotency Key — 客户端命令重试键

```text
idempotencyKey = idem_<uuid>
```

由 Admin Console/调用方为 Publish 等可重试命令生成。它不是业务实体 ID，仅在 retention window 内保证同一命令不被重复执行。

### 6.5 AgentSpace — 用户托管执行空间（S1）

```text
agentSpaceId = spc_<uuid>
```

Agent Space 是 User-owned Runtime Instance。`diskId/runtimeId` 是底层运行引擎 external identifier，迁移底层引擎不改变 `agentSpaceId`。

### 6.6 AgentDefinition — Agent 稳定定义（S2）

```text
agentId = agt_<uuid>
```

AgentDefinition 描述“哪个 Agent”：instructions、model/tool policy、capability refs、workspace policy 等。`Remote Agent` 只是该 Definition 被暴露为 delegation target 的角色，不创建第二套 `agentId`。

### 6.7 AgentRun — 一次 Agent 执行（S2）

```text
runId = run_<uuid>
```

一个 `agentId` 可产生任意多个 `runId`。Phase 1 Remote Delegation 使用 request-scoped AgentRun。

### 6.8 AgentRuntime — Agent 执行子系统（S2+）

Agent Runtime 是逻辑子系统名称，不对应新的业务 Entity ID。它负责 AgentRun 生命周期，并在 Phase 2 扩展 durable AgentInstance、checkpoint/resume 和调度。

S2 初版允许与 Agent Space 服务同进程实现；是否拆独立服务由实际调度/故障域决定。

### 6.9 AgentFleet — 企业持续运行 Agent 集群（Phase 2）

```text
fleetId = flt_<uuid>
```

Agent Fleet 是一个企业管理的 AgentInstance 集合，拥有部署、并发、资源、Trigger、运行策略和观测状态。它不是一个永不结束的 AgentRun。

### 6.10 AgentInstance — 持久 Agent 实例（Phase 2）

```text
agentInstanceId = agi_<uuid>
```

表示某 AgentDefinition 在 Fleet 中的长期部署身份。进程/POD/VM 重启不改变 agentInstanceId；单次工作仍产生独立 `runId`。

### 6.11 AgentTrigger — Agent 唤醒/启动规则（Phase 2）

```text
triggerId = trg_<uuid>
```

可表示 schedule、webhook、business event 等触发方式。Trigger 是 Definition，不把外部 event id 当作 triggerId。

### 6.12 EnterpriseConnector — 企业业务系统连接（Phase 2）

```text
connectorId = con_<uuid>
```

用于 ERP、CRM、数据库、内部 API/SaaS 等数据/动作接入。Connector 与 Upstream Adapter 不同：前者面向 Agent 的业务上下文/动作，后者面向 AI/Speech/MCP Provider 协议兼容。

### 6.13 Artifact — 运行产物（S2+）

```text
artifactId = art_<uuid>
```

path/object key 不是 Artifact identity；移动底层存储不改变 artifactId。

### 6.14 RuntimeHookDefinition — Hook 定义（S3）

```text
hookId = hok_<uuid>
```

### 6.15 RuntimeHookInvocation — 一次 Hook 调用（S3）

```text
hookInvocationId = hki_<uuid>
```

用于 Audit/Usage/timeout 排障，可关联 interactionId/requestId/runId。

## 7. Metering / Audit 术语

### 7.1 UsageEvent — 用量事实

归一化进入 Usage Ledger 后：

```text
usageEventId = usg_<uuid>
```

Relay RequestUsage 的去重自然键仍是 `requestId`；Usage Ledger 自身需要独立 usageEventId，以容纳同一 request 的多个 semantic meter/后续补录。

### 7.2 External Source Event ID

Provider/Adapter webhook/log 自己返回的 `sourceEventId` 是**外部 opaque identifier**，不添加 MEASIX prefix、不重新生成。数据库必须同时保存 source namespace/upstreamId，避免不同 Provider 冲突。

### 7.3 Audit Event（Phase 2）

当 Audit 成为独立持久事实时：

```text
auditEventId = aud_<uuid>
```

Audit 不以 requestId/activationId 代替自身 identity，但应关联它们。

---

## 8. User Data Plane（Phase 2）标识策略

Android 已经存在 Conversation/MessageNode 等本地稳定 ID。Phase 2 上云同步的第一原则是：

> **已有本地用户数据 ID 不因接入 MEASIX Cloud 而重写。**

因此 Phase 2 wire model 中：

```text
conversationId
messageNodeId
attachmentId
```

必须视为 opaque stable string，并接受既有 Android ID。

对于未来由云端新建的对象，预留 MEASIX namespace：

```text
conversationId = cnv_<uuid>
messageNodeId   = mnd_<uuid>
attachmentId    = att_<uuid>
```

但同步协议不能要求已有 Android 数据批量换成这些 prefix。Origin-created ID 必须端到端保留。

### 8.1 EnterpriseAssistantMemory — 企业助手运行记忆

**定义**：企业下发的 `ManagedAssistantDefinition` 在 Enterprise Realm 真实运行中产生的、可持续更新的用户/助手经验状态。它是 User Data，不是 Managed Definition，不进入 Managed Snapshot。

已有 Android memory ID 按 opaque stable string 保留；云端新建记忆可使用：

```text
memoryId = mem_<uuid>
```

记忆必须可归属到 `deploymentId + userId + assistantDefinitionId`，并保留 source conversation/message/tool/run 等 provenance。一条记忆被同步到企业后仍然是运行记忆，不因上传自动成为向其他用户发布的共享经验。

### 8.2 ExperienceContribution — 经验贡献

```text
experienceContributionId = xct_<uuid>
```

ExperienceContribution 是由一条或多条企业助手记忆、会话、工具结果、AgentRun 或场景结果提取的可复用经验候选。它有独立审核/评估/提升生命周期；只有被提升为 ManagedAssistantDefinition、ManagedSkillDefinition、AssistantStarterDefinition 或其他 Managed Definition 并进入新 ManagedRelease 后，才成为正式可下发经验。

### 8.3 EnterpriseActivity — 企业定向触达事件

```text
enterpriseActivityId = eac_<uuid>
```

EnterpriseActivity 表示向用户/组/设备定向投递的通知、Starter、任务或结果提醒。它可引用 `starterId`、`agentId`、`assistantDefinitionId` 或其他资源，但投递事件本身不改变 Definition。Push/SSE 只是到达优化，不承担 Managed State correctness。

### 8.4 EnterpriseUpdate — 企业动态

```text
enterpriseUpdateId = eup_<uuid>
```

EnterpriseUpdate 是企业持续发布、供 Portal 用户阅读并可由企业能力查询的动态内容。正式产品名称为“企业动态”，不等同于要求确认/回执的正式公告，也不局限于宣传新闻。它有独立 Feed revision/ETag，不进入 Managed Snapshot；发布或撤回企业动态不改变 `managedGeneration`。

---

## 9. ID 格式与生成标准

### 9.1 新 MEASIX ID 的统一格式

所有本文新定义的 MEASIX ID 使用：

```text
<prefix>_<canonical-lowercase-UUIDv4>
```

示例：

```text
dep_550e8400-e29b-41d4-a716-446655440000
usr_9cc8dca8-58e9-4a4d-87df-a40c74fc87fe
mdl_3b49e4af-b994-4fa5-830d-c8ab52f66f83
req_e8bf5f25-ec8d-46f1-9bbc-948ab8af2612
```

UUID 使用 RFC 9562 定义的 UUIDv4。选择 v4 的原因：跨 Go/JVM/Browser 原生支持成熟、不暴露创建时间、不要求中心分配、不同组件可安全独立生成。

**禁止依赖 UUID 或完整 ID 的字典序推断创建顺序。** 顺序必须使用 `createdAt` / revision/generation。

### 9.2 生成责任

- Control Hub：生成所有 Identity、Security、Operational、Release/Activation 及其他拥有独立服务端生命周期的 entity IDs；
- Runtime Relay：只生成 `requestId` 等它拥有的 Runtime correlation IDs；
- Android：生成 `installationId`、`interactionId` 及既有本地 User Data IDs；
- Admin Console：生成 `idempotencyKey`；另外允许在 **Managed Draft typed aggregate** 内为新建的 `prv_* / mdl_* / tts_* / asr_* / mcp_* / pol_* / rte_*`、S0.2 `asd_* / str_*`、S0.3 `twg_*` propose candidate stable ID；S0.3 Catalog review 可按同样 Hub validation 规则 propose `gtl_*`，不扩张为通用服务端实体 ID 分配权；
- S1/S2/S3 Runtime owner service：生成自己拥有的 Space/Run/Hook Invocation ID。

Draft candidate ID 的特殊规则：

1. 浏览器使用 `crypto.randomUUID()` + canonical prefix 生成，目的是让同一次完整 Draft 编辑中的新对象在首次 Save 前即可形成稳定引用；
2. candidate ID 在 Control Hub 成功校验并接受 Draft PUT 前不具有服务端权威性；Hub 必须校验 prefix、UUID、类型、唯一性、引用闭合和与既有 ID 的冲突；
3. 一旦 Hub 接受，该 ID 即成为 Definition/RuntimeRoute 的长期 stable ID，后续 Save/Release/Republish 不得重写；
4. 该例外**不适用**于 `dep_* / usr_* / dev_* / enr_* / ses_* / drf_* / rel_* / ups_* / sec_* / act_* / usg_* / prc_*` 等服务端生命周期对象；
5. UI 生成 ID 不赋予任何授权能力。对象 authority、持久化和生命周期仍由 Control Hub 决定。

因此长期原则不是“所有 ID 必须由单一进程生成”，而是：**拥有对象生命周期的 authority 必须决定 ID 是否成立；跨组件不能重新生成已经成立的 stable ID。** S0 Draft aggregate children 允许 caller-proposed ID，是为了避免额外的 ID-allocation API 和临时引用协议。

### 9.3 全局唯一与逻辑 scope

ID payload 按全局唯一设计，即使业务逻辑仍属于某个 Deployment/Tenant。

例如：

```text
usr_* globally unique
but ownership scope = deploymentId / future tenantId
```

这样 Phase 3 增加 Tenant 时不需要给 Phase 1 旧数据重新编号。

### 9.4 不可变、不可复用

以下行为全部禁止：

- 删除 User 后将同一个 `usr_*` 分配给另一个人；
- Rename Resource 时生成新 ID；
- Provider model name 改变时重写 modelId；
- Device 改 display name 时重写 deviceId；
- Republish 时复用旧 releaseId；
- 数据库 restore 后重新生成实体 ID；
- 把数据库自增 rowid 暴露为平台 stable ID。

### 9.5 Prefix 是类型保护，不是业务数据

Prefix 用于日志可读性、输入校验和误用防护，但不得编码：

```text
tenant
department
region
user name
provider name
model name
日期
权限
```

例如 `mdl_*` 表示该 ID 的类型是 Managed Model，但 ID 本身不携带“GPT/OpenAI/Production”等可变信息。

### 9.6 Wire/DB 类型

Wire：统一 JSON string。  
SQLite/Ent：核心 stable ID 使用 TEXT/string primary key 或 unique immutable key；不向 wire 暴露 SQLite rowid/autoincrement integer。

OpenAPI 对各 ID 建立独立 schema，例如：

```text
DeploymentId
UserId
DeviceId
ModelId
RuntimeResourceId
InteractionId
RequestId
```

不要在所有接口里退化成无约束的普通 `string`。

---

## 10. External Identifier / Alias 规则

长期系统一定会接触大量不是 MEASIX 生成的标识：

```text
OIDC issuer/sub
SAML NameID
SCIM externalId
Provider model key
CLIProxyAPI model name
runtime engine diskId/runtimeId
cloud region code
provider request id
```

统一规则：

1. 保持 opaque，不重新解释其内部结构；
2. 用明确字段名说明来源，例如 `upstreamModelKey`，禁止叫含糊的 `modelId`；
3. 外部 ID 变化时，通过 mapping 更新，不重写 MEASIX stable ID；
4. 若需要唯一约束，使用 `(sourceNamespace, externalId)`；
5. 外部 ID 不作为跨组件授权主键，除非对应 adapter contract 明确证明其 authority。

---

### 10.1 platformUrl — 客户端外部入口定位

`platformUrl` 是 Android/未来客户端保存的唯一平台入口 origin，例如：

```text
https://agent.company.com
```

它不是 Deployment identity，也不包含 `/api/...` 或 `/runtime/...` 子路径。客户端在该 origin 下发现/调用：

```text
/.well-known/measix
/api/client/v1/*
/runtime/v1/*
```

域名、Ingress 或部署拓扑变化可以修改 `platformUrl`，但不得因此重写 `deploymentId`、`userId` 或其他 stable ID。

### 10.2 本地示例来源命名空间

客户端对真实平台的来源绑定包含已验证 HTTPS origin 与 deploymentId；origin 规范化不改变已登记绑定，名称、用户显示名、登录 token 与 Session 轮换不产生新来源。新 origin 的相同 deploymentId 不能仅凭 ID 或名称自动合并旧数据；来源迁移不属于 S0.2 自动接入。客户端持久区分本地与平台来源，内部来源标识不是新增平台 wire ID，不从不受信扫码字段动态取得平台授权。重登复用同一已验证来源但取得新的 Session，旧页面授权不能复活。

本地示例不具有真实平台认证身份。原生保存的来源命名空间用于区分本地示例与已验证平台；本地来源标识 `sourceNamespace` 仅从应用已安装来源目录解析，不是网络地址。即使 Deployment/User ID 字符串相同，两种来源的绑定、偏好和运行数据也不相同。该本地区分不修改平台 wire ID，不随名称匹配自动合并；切换到真实平台须重新接入。接入资料字段与验证规则由 Control Protocol §8 定义。

## 11. Version / Revision / Sequence 的固定语义

### 11.1 managedGeneration

```text
scope: deploymentId
start: 1 for first published Release
0: reserved for "no published managed release"
monotonic: yes
reuse: never
```

它表示客户端 Managed Snapshot/Release 顺序，不是 request generation，也不是 ID。

### 11.2 controlRevision

```text
scope: deployment runtime-control domain
start: 1 when first Relay control becomes valid
monotonic: yes
```

Secret rotation、revoke、Upstream operational change 可以只增加 controlRevision。

### 11.3 gatewayControlRevision

```text
scope: deployment gateway-control domain
start: 1 when first Gateway control becomes valid
monotonic: yes
```

Tool Integration credential/endpoint operational change、Gateway limit 或 applied control correction 可以只增加 `gatewayControlRevision`；client-visible surface/Catalog/guidance 变化还必须创建新 `managedGeneration`。

### 11.4 managedStateRevision

```text
scope: deploymentId
monotonic: yes
```

用于 Android 判断 Managed State 是否更新。它是状态变化序号，不意味着存在可重放的事件日志。

### 11.5 draftRevision

```text
scope: draftId
monotonic: yes
```

只负责 optimistic concurrency。

### 11.6 Hash

`snapshotHash` / `bundleHash` 是内容完整性摘要。Hash 变化说明内容变化，但 Hash 不是对象 identity，也不参与 ownership。

---

## 12. S0 / Phase 1 落地顺序

### I0 — Identifier Foundation

必须先完成：

- 本文进入 OpenAPI/schema 权威链；
- Go `platformid` 生成/校验 utility；
- `github.com/google/uuid` 用于 Go UUIDv4；
- Android 使用 `java.util.UUID.randomUUID()` 生成 `ins_*` / `int_*`，不新增 ID library；
- Browser 使用 `crypto.randomUUID()` 生成 `idem_*`，并仅在 Draft editor 中生成允许的 Definition/Route candidate IDs；
- Control Hub 对 caller-proposed Draft IDs 做与 `platformid` 同源的严格 validation；
- OpenAPI 为 stable IDs 提供 typed schema/pattern；
- contract tests 验证 prefix、UUID、大小写、错误类型、Draft candidate collision/reference 行为。

### I1 — Identity IDs

落地：

```text
dep_*  Deployment
usr_*  EnterpriseUser
dev_*  Device
enr_*  Enrollment
ses_*  Session
ins_*  Android installation
```

Administrator 作为 User/Principal role，不创建 `adm_*`。

### I2 — Managed Definition IDs

落地：

```text
prv_*  Managed Provider Definition
mdl_*  Managed Model
tts_*  Managed TTS
asr_*  Managed ASR
mcp_*  Managed MCP Server
twg_*  Enterprise Tool Gateway logical resource
tin_*  Gateway Tool Integration
gtl_*  Published Gateway Tool Definition
pol_*  Managed Policy
drf_*  Managed Draft
rel_*  Managed Release
ups_*  Upstream
sec_*  Managed Secret
rte_*  Runtime Route
```

同时固化 `upstreamModelKey` 等 external identifier 命名。

### I3 — Runtime Control IDs

落地：

```text
act_*  Activation
req_*  Runtime Request
```

并正式启用：

```text
managedGeneration
controlRevision
gatewayControlRevision
managedStateRevision
```

### I4 — Client Runtime IDs

正式贯穿所有 Managed Runtime：

```text
int_* interactionId
resourceId = mdl_*/tts_*/asr_*/mcp_*/twg_*
req_* response correlation
```

### I5 — Metering IDs

落地：

```text
usg_* Usage Ledger event
prc_* Pricing Rule
```

RequestUsage 继续以 `req_*` 作为 request-level idempotency/correlation key。

---

## 13. Phase 1 后续与长期保留 namespace

### S1

```text
spc_* Agent Space
```

### S2

```text
agt_* Agent Definition
run_* Agent Run
art_* Artifact
```

### S3

```text
hok_* Runtime Hook Definition
hki_* Runtime Hook Invocation
```

### Phase 2

```text
org_* Organization
grp_* Group
mbr_* Membership
aud_* Audit Event
flt_* Agent Fleet
agi_* Agent Instance
trg_* Agent Trigger
con_* Enterprise Connector
cnv_* / mnd_* / att_* reserved for cloud-created User Data entities
mem_* cloud-created Enterprise Assistant Memory
xct_* Experience Contribution
eac_* Enterprise Activity
```

S0.2 首次交付以下企业经验与动态 namespace：

```text
asd_* Managed Assistant Definition
str_* Assistant Starter Definition
eup_* Enterprise Update
```

`skl_* Managed Skill Definition` 保留为 Phase 1 future，不进入 S0.2 Exit。

已有 Android Conversation/User Data ID 不重写。

### Phase 3

```text
tnt_* Tenant
```

Tenant 作为新的上位 isolation scope 加入，不替换 `dep_* / usr_* / capability / agent` IDs。

## 14. 组件责任矩阵

| 标识/术语 | Owner / 生成方 | 首次阶段 | 主要消费者 |
|---|---|---|---|
| `deploymentId dep_*` | Control Hub | I1 | Android/Admin/Relay/Usage |
| `userId usr_*` | Control Hub | I1 | 全平台 |
| `deviceId dev_*` | Control Hub | I1 | Android/Relay/Admin |
| `installationId ins_*` | Android Client | I1 | Enrollment/诊断 |
| `enrollmentId enr_*` | Control Hub | I1 | Admin/Hub |
| `sessionId ses_*` | Control Hub | I1 | Android/Admin/Relay |
| `providerId prv_*` | Hub authority；Admin Draft 可 propose | I2 | Snapshot/Android |
| `modelId mdl_*` | Hub authority；Admin Draft 可 propose | I2 | Android/Relay/Usage/Admin |
| `ttsId/asrId/mcpServerId` | Hub authority；Admin Draft 可 propose | I2 | Android/Relay/Usage/Admin |
| `toolGatewayId twg_*` | Hub authority；Admin Draft 可 propose | S0.3 | Snapshot/Android/Relay/Gateway/Usage |
| `toolIntegrationId tin_*` | Control Hub | S0.3 | Hub/Gateway/Admin only |
| `gatewayToolId gtl_*` | Hub authority；Admin Catalog review 可 propose | S0.3 | Hub/Gateway/Admin/Audit |
| `policyId pol_*` | Hub authority；Admin Draft 可 propose | I2 | Release/Snapshot |
| `draftId drf_*` | Control Hub | I2 | Admin/Hub |
| `releaseId rel_*` | Control Hub | I2/I3 | Admin/Android/Hub |
| `upstreamId ups_*` | Control Hub | I2 | Relay/Admin/Usage |
| `secretId sec_*` | Control Hub | I2 | Hub/Relay only |
| `runtimeRouteId rte_*` | Hub authority；Admin Draft 可 propose | I2 | **Relay only** |
| `activationId act_*` | Control Hub | I3 | Hub/Admin diagnostics |
| `requestId req_*` | Runtime Relay | I3 | Android/Hub/Adapter/Usage |
| `interactionId int_*` | Android/client | I4 | Relay/Usage/Logs |
| `idempotencyKey idem_*` | Admin/client caller | I3 | Hub dedup |
| `usageEventId usg_*` | Control Hub metering | I5 | Usage Ledger |
| `pricingRuleId prc_*` | Control Hub | I5 | Cost |
| `agentSpaceId spc_*` | Agent Space owner | S1 | Hub/Agent Runtime/Usage |
| `agentId agt_*` | Control Hub Agent Catalog | S2 | Android/Agent Runtime |
| `runId run_*` | Agent Runtime | S2 | Android/Usage/Artifacts |
| `artifactId art_*` | Agent Runtime/Space | S2 | Android/User Sync |
| `hookId hok_*` | Control Hub | S3 | Runtime/Android |
| `hookInvocationId hki_*` | Hook runtime | S3 | Audit/Usage |
| `fleetId flt_*` | Control Hub/Fleet management | Phase 2 | Agent Runtime/Admin |
| `agentInstanceId agi_*` | Agent Runtime | Phase 2 | Fleet/Usage/Audit |
| `triggerId trg_*` | Control Hub | Phase 2 | Agent Runtime |
| `connectorId con_*` | Control Hub | Phase 2 | Agent Runtime/Fleet |
| `assistantDefinitionId asd_*` | Hub authority；Admin Draft 可 propose | S0.2 | Release/Snapshot/Android/User Sync |
| `skillId skl_*` | Control Hub Experience Catalog | Phase 1 future | Release/Snapshot/Android/Agent Runtime |
| `starterId str_*` | Hub authority；Admin Draft 可 propose | S0.2 | Release/Snapshot/Android；未来 Enterprise Activity 引用 |
| `enterpriseUpdateId eup_*` | Control Hub Enterprise Update authority | S0.2 | Client/Portal Feed、Android cache；S0.3 Gateway private query |
| `memoryId mem_*` | origin client or User Sync authority | Phase 2 | Android/User Sync/Experience Curation |
| `experienceContributionId xct_*` | Experience Curation | Phase 2 | Admin/Review/Promotion/Audit |
| `enterpriseActivityId eac_*` | Control Hub engagement authority | Phase 2 | Portal/Android/Audit |
| `tenantId tnt_*` | Multi-tenant Control Plane | Phase 3 | 全平台 scope |

## 15. 代码与评审不变量

以下作为长期 code review / AI coding guardrail：

1. 新核心实体必须先在本文注册术语与 ID namespace，再进入代码；
2. 不得使用 displayName/email/model name/base URL 作为关系主键；
3. 不得跨组件重新生成已有 entity ID；
4. 不得把外部 Provider ID 命名成 MEASIX stable ID；
5. 不得用 integer auto-increment wire ID 替代 platform ID；
6. 不得让 `resourceId` 失去资源类型约束；
7. 不得把 generation/revision/hash 命名为 `*Id`；
8. Rename/Move/Secret rotation/Provider migration 默认不改变 stable ID；
9. Hard delete 后 ID 永不复用；
10. Audit/Usage/Logs 优先记录 stable ID；生产日志不得因此附带用户名、邮箱或其他敏感 display name。受授权 Admin 展示可按需解析名称，日志字段遵循 S0 结构化日志/脱敏合同；
11. OpenAPI、数据库 Schema、日志字段和 UI route parameter 必须使用本文的 canonical field name；
12. 任何需要改变既有 ID 语义的设计都属于 Platform Architecture Breaking Change，必须先修改本文并给出迁移策略。

---

## 16. 一句话识别规则

```text
企业现在是谁？          deploymentId；Phase 3 再额外有 tenantId
用户是谁？              userId
哪台注册设备？          deviceId
哪个客户端安装？        installationId（非授权身份）
哪个逻辑模型？          modelId
Provider 实际模型名？    upstreamModelKey（外部标识）
哪个执行后端？          upstreamId
走哪条服务端路由？      runtimeRouteId
哪次不可变发布？        releaseId
客户端当前第几版？      managedGeneration（不是 ID）
哪次顶层交互？          interactionId
哪一个 HTTP Runtime？   requestId
哪次发布事务？          activationId
哪个云端 Agent？        agentId
哪次 Agent 执行？       runId
哪个用户空间？          agentSpaceId
当前 Android 属于哪个域？ ClientRealm；ENTERPRISE 时携带 deploymentId
哪个企业助手定义？  assistantDefinitionId
哪个企业 Skill？       skillId
哪条助手运行记忆？    memoryId（已有 Android ID 可保留）
哪次经验贡献？          experienceContributionId
```

这组规则作为 MEASIX 从 S0 到多租户阶段的长期语言基础。
