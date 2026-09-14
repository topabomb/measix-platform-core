# MEASIX Agent Platform 路线图与架构边界

> 状态：Architecture Baseline / 架构基线  
> 版本：2026-08-31
> 平台：**MEASIX Agent Platform（MEASIX 智能体平台）**  
> 文档职责：定义平台长期定位、核心领域、短/中/长期阶段、长期不变边界和演进方向；不定义具体 REST endpoint、数据库表或单组件内部实现。

## 1. 文档体系与权威关系

架构文档按“战略 → 长期术语/身份 → 阶段架构 → 阶段实施 → 跨组件契约 → 单组件规格”逐层细化：

```text
measix-agent-platform-roadmap.md
  ├─ measix-platform-terminology-and-identifier-contract.md
  ├─ measix-enterprise-experience-lifecycle-architecture.md
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

权威关系：

1. 本文只决定长期方向、阶段边界和不可破坏的架构原则；
2. `measix-platform-terminology-and-identifier-contract.md` 决定跨 Phase 的正式术语、子系统名称、stable ID namespace 和字段语义；
3. `measix-enterprise-experience-lifecycle-architecture.md` 决定企业经验从发布、运行、记忆回流、提炼到再发布的跨 Phase 生命周期；
4. `measix-runtime-foundation-architecture.md` 决定 Phase 1 的系统组成、职责边界和 S0–S3 演进；
5. `measix-s0-foundation-contract-spec.md` 决定 S0 产品闭环与范围；
6. `measix-s0-implementation-decision.md` 决定 S0 实施顺序、技术方向和跨组件依赖边界；具体依赖版本由实现仓库 pin；
7. `measix-s0-control-protocol.md` 决定 S0 跨组件 wire/state/一致性语义；
8. 单组件责任/不变量与必测行为由组件规格定义；具体结构、接口代码、命令和执行证据由实现仓库维护。

**品牌与组件命名分离**：`MEASIX` 是平台品牌，只用于平台/阶段的完整产品名称和文档命名空间；组件正式名称不重复冠以品牌名。S0 起固定使用 `Control Hub`、`Runtime Relay`、`Enterprise Tool Gateway`、`Admin Console`、`Android Client`、`Agent Space`、`Agent Runtime` 等职责型专名。

GitHub `measix-architecture` 仓库的默认分支是正式架构文档的唯一权威来源；Google Drive 不再作为架构文档的维护、同步或归档目标。

## 2. 平台定位与总体目标

MEASIX 当前以 Android 客户端（现 `rikkahub_mcp`）为主要用户入口，已有本地 Provider/Model、Assistant、MCP、Workspace、Conversation、Tool Loop、子助手和本地持久化能力。

企业化目标不是把 Android 降级成云端薄客户端，也不是简单增加管理后台，而是逐步建立一套可治理、可执行、可持续运行的企业 Agent 基础设施。

长期平台必须同时满足：

- **Local-first Runtime**：脱离企业绑定后，本地能力仍是完整产品；
- **Capability Governance**：企业可以明确用户/Agent 被允许使用什么；
- **Hosted Workspace**：企业可以为用户提供持久、隔离的 Agent Space；
- **Agent Runtime**：云端能够执行 request-scoped 与 durable Agent；
- **Remote Delegation**：Android/Agent 可把任务委派给受管 Agent；
- **Enterprise Agent Fleet**：中期支持企业自己持续运行、可恢复、可调度的 Agent 集群；
- **Runtime Intervention**：企业通过确定 Hook Point 介入运行，而不是远程修改客户端数据库；
- **Enterprise Integration**：ERP、CRM 等业务系统通过明确 Connector 边界被 Agent 安全访问；
- **Identity & Governance**：所有请求、Run、Agent Instance 和资源都归属于稳定 Principal；
- **Usage Ledger**：模型、语音、Agent、计算与存储用量可追踪；
- **User Sync**：中期独立建立 Conversation/Message/Attachment 等用户数据同步；
- **Enterprise Experience Evolution**：企业可以把 Android、Agent Runtime 和 Agent Fleet 在真实业务中产生的助手记忆、流程和方法，持续提炼为可评估、可发布、可再使用的企业经验；
- **Multi-tenancy**：长期演进到多企业、多区域和规模化运营。

其中 `Upstream Adapter` 与 `Enterprise Connector` 是两类不同的外部集成：前者解决 AI/Speech/MCP Provider 协议兼容，后者连接 ERP/CRM/数据库/SaaS 等企业业务系统。

## 3. 七个长期架构锚点

长期架构围绕七个领域锚点组织。组件可以合并或拆分，但这些责任边界不随部署形态改变。

### 3.1 Managed Capability — 企业定义“能用什么”

企业发布 Model、TTS、ASR、MCP、后续 Search、Agent Definition、Runtime Hook Definition、Agent Space Policy 等 Definition/Policy。客户端通过版本化 Snapshot 消费；运行实例不进入 Snapshot。

### 3.2 Principal — “谁在执行”

所有企业请求和 Agent Run 都必须解析到稳定 Principal。长期主体类别收敛为：

```text
USER | AGENT | SYSTEM
```

Device 是 User 请求的设备上下文，不单独成为业务所有者；Agent Run/Agent Instance 作为 `AGENT` 主体进入授权、Usage 和 Audit。

### 3.3 Agent Space — “在哪里执行、保存什么”

Agent Space 是用户级持久、隔离的 Hosted Workspace/Runtime Resource，拥有独立 disk 与计算边界。`AgentSpacePolicy` 是控制面定义，`AgentSpace` 是运行实例。

### 3.4 Agent Runtime — “Agent 如何被执行”

Agent Runtime 是执行 Agent Definition 的运行原语，负责 Run 生命周期、取消、超时、状态、产物、后续 checkpoint/resume 和 durable execution。

```text
AgentDefinition
      ↓
Agent Runtime
      ├─ AgentRun            # 一次执行
      └─ AgentInstance       # Phase 2 durable instance
```

`Remote Agent` 不是另一套 Runtime，而是 **Agent Definition 被暴露为 delegation target 的角色**。`Agent Fleet` 则是 Phase 2 建立在 Agent Runtime 之上的持续运行 Agent 集群。

### 3.5 Runtime Hook — “企业怎样介入运行”

企业只通过确定 Hook Point 做上下文注入、工具治理、审批、安全/DLP 等介入。Hook 不直接远程写 Android Room/Settings，也不与 Agent Runtime 本身混为一体。

### 3.6 Usage Ledger — “执行消耗了什么”

Usage Ledger 是 append-only 用量事实基础，统一承载模型 token、TTS/ASR、Agent Run、Agent Space compute/storage、Hook invocation 等，并服务 Cost、Quota、Audit 与未来 Billing。

### 3.7 Enterprise Experience — “企业学到了什么、如何继续进化”

Enterprise Experience 是企业助手、Skill、初始记忆/知识、场景模板和已验证工作方法的长期领域。它不仅接收企业正式发布的经验定义，也承接 Android、Agent Runtime 和 Agent Fleet 在真实运行中产生的助手记忆与经验贡献，经汇总、审核、评估和提升后再进入 Managed Release。

详细对象、ownership、状态和双向流程由 `measix-enterprise-experience-lifecycle-architecture.md` 定义。

```text
Managed Capability  = 允许什么
Principal           = 谁在执行
Agent Space         = 在哪里执行/持久化
Agent Runtime       = Agent 如何执行
Runtime Hook        = 企业如何介入
Usage Ledger        = 消耗了什么
Enterprise Experience = 学到了什么、如何再发布
```

## 4. 长期 Plane 与子系统架构

Plane 是责任边界，不等于“每个 Plane 一个微服务”。Phase 1 以少量进程实现；只有负载、故障域或组织边界真实需要时再拆分。

```text
Experience
  Android Client / Enterprise Portal / Admin Console
  Enterprise Experience Catalog / Curation
                 │
                 ▼
Identity & Control Plane
  Control Hub
  ├─ Identity Directory
  ├─ Capability Catalog / Policy / Release
  ├─ Agent Catalog (S2+)
  ├─ Runtime Desired State
  └─ Usage Ledger / Cost
                 │
                 ├───────────────────────┐
                 ▼                       ▼
Runtime Plane                         Integration Boundary
  Runtime Relay                        Upstream Adapter
  Enterprise Tool Gateway              Tool Integration (MCP, S0.3)
  Agent Space                          Enterprise Connector (Phase 2)
  Agent Runtime
  Runtime Hook
  Agent Fleet (Phase 2, built on Agent Runtime)
                 │
                 ▼
Phase 2: User Sync
Conversation / Message / Attachment / Preference
Enterprise Assistant Memory / Experience Contribution

Phase 3: Tenant / Region / Billing / Federation
```

固定的子系统名称：

| 名称 | 稳定职责 | 首次落地 |
|---|---|---|
| Control Hub | 控制状态真源、Identity、Catalog、Release、Desired Runtime State、Usage | S0 |
| Runtime Relay | Runtime admission、授权、路由、credential、透明 L7 relay、request metering | S0 |
| Enterprise Tool Gateway | 固定 Meta Tool surface、Published Tool Catalog discovery、toolRef/schema enforcement、受治理代理执行 | S0.3 |
| Admin Console | 企业管理体验；只调用 Control Hub | S0 |
| Android Client | 本地 Runtime + 企业绑定/受管能力接入 | S0 |
| Enterprise Portal | 企业用户工作台；S0.2 交付 Native Host、状态、操作、企业动态及受限拍照/录音，后续扩展同步/备份/经验/任务 | S0.2 MVP |
| Agent Space | 用户级隔离 workspace/runtime resource | S1 |
| Agent Runtime | Agent Definition/Run 的执行生命周期 | S2 |
| Remote Agent | Agent Definition 作为 delegation target 的角色 | S2 |
| Runtime Hook | 运行介入机制 | S3 |
| Agent Fleet | 企业持续运行的 Agent Instance 集群 | Phase 2 |
| Enterprise Connector | ERP/CRM/DB/SaaS 的数据与动作接入 | Phase 2 |
| User Sync | 用户数据增量同步 | Phase 2 |

泛称 `server`、`gateway`、`worker`、`proxy` 只用于解释实现角色；完整专名 `Enterprise Tool Gateway` 是已注册的 S0.3 子系统，不能简称为与 Runtime Relay 混淆的通用 `Gateway`。

## 5. 长期不变的架构原则

### 5.1 用户配置、用户偏好与企业下发配置分别归属

企业状态不能覆盖用户原配置。配置具有三类逻辑归属：用户配置保存用户 A/B/C 定义、个人资料和个人备份配置；用户偏好保存公用显示/操作偏好及各域的选择和允许调整的开关；企业下发配置按企业保存受管 A/B/C 定义与 Policy。

```text
Effective Runtime(PERSONAL) = Built-in + User Configuration + Personal Preferences
Effective Runtime(ENTERPRISE, deploymentId)
  = Managed Snapshot + policy-allowed User Configuration + Enterprise Preferences
```

用户资源只保存一份，获准后企业域直接引用原定义并使用其用户凭据，不复制企业版本、不上传凭据、不注入企业 Session。来源与稳定 ID 一起确定资源引用，不能以 Managed same-ID 覆盖用户定义。企业策略仅决定企业域可用性，不删除、禁用或改写用户原配置。

Personal 不消费企业内容；企业域按类别对用户及可选 Built-in 配置执行准入。当前五项控制清单之外的用户配置按生命周期架构 §4.1 的独立决策开放；五项全部 false 不代表全部 A/B/C 禁用。不可移除的安全/导航等宿主功能不属于 A/B/C。资源定义复用不隐式共享聊天、运行记忆或文件；运行数据按域和主体隔离，显式共享 Workspace 的访问边界另行标明，解除绑定不删除 Personal 数据。企业运行数据退出后的保留/导出策略另行明确。

生效配置是从上述 owner 派生的只读快照，不是第四份持久化真源。UI、写入命令与执行遵循同一解析规则；运行开始捕获配置与归属，切域和后台更新不得在运行中静默替换。三类归属不规定文件或 DataStore 数量；身份、Session、同步、运行数据和恢复状态保留各自 owner。详细规则由 Enterprise Experience 生命周期架构 §4 与 Android 接入合同承接。

企业接入、状态与空间切换属于正式 Android 产品入口，Debug/Release 均可使用。本地示例企业服务允许在生产平台接入前验证同一套配置、页面和运行机制，持续显示来源并与真实企业身份隔离；它不替代 S0.2 或后续阶段的真实平台 Freeze。具体交付边界由 S0.2 和 Android 接入合同定义。

### 5.2 Definition 与 Runtime Instance 分离

- `AgentSpacePolicy` ≠ `AgentSpace`；
- `AgentDefinition` ≠ `AgentRun`；
- `RuntimeHookDefinition` ≠ Hook Invocation；
- Managed Resource ≠ Runtime Route / Upstream Instance。

Managed Snapshot 只承载 Definition/Policy，不承载用户级运行实例。

### 5.3 Control Plane 与 Runtime Plane 分离

- Control Plane 决定谁、能用什么、当前发布什么、Relay/Runtime 应执行什么；
- Runtime Plane 承担真实执行、路由、流式传输和运行实例。

管理 CRUD、发布和数据库事务不能直接处于模型/TTS/ASR 大流量路径中。

### 5.4 Managed Delivery 与 User Data Sync 分离

企业配置采用不可变 Snapshot + 单调 `managedGeneration` + 原子发布；用户数据同步在 Phase 2 采用记录级/增量同步协议。两者不能复用同一套版本和冲突模型。

灾备 ZIP/WebDAV/S3 Backup 也不等同于 User Data Sync。

企业经验进化同时使用两条链路：企业助手/Skill/场景等正式经验通过 Managed Delivery 下发；企业助手运行产生的记忆和 Experience Contribution 通过 User Sync 回传。二者共同形成一个业务闭环，但不得共用同一套版本、冲突或发布模型。

### 5.5 Stable Identity 从 Phase 1 开始

Phase 1 不提前引入复杂 Tenant/Organization，但必须有稳定平台 ID（Phase 1 由 Control Hub 签发）：Deployment、EnterpriseUser、Device、Session。Email、username、Android ID、IMEI 等都不是关系身份主键。

长期身份演进：

```text
Phase 1: Deployment ≈ Enterprise
Phase 2: Enterprise + Organization/Group/Membership
Phase 3: Tenant → Deployment / Region / Users
```

外部 OIDC/SAML/SCIM 只改变“如何映射到 EnterpriseUser”，不改变资源 ownership ID。

### 5.6 Android 保持完整 Runtime

现有：

```text
ChatService → ConversationSession → GenerationHandler → Tool Loop
```

继续作为 Android 运行主链。企业能力通过 Effective Runtime、ManagedStateGuard、Runtime Relay、Remote Delegation 和 Hook 边界接入，不把本地生成主链搬到 Control Hub。

### 5.7 Provider Compatibility 与 Enterprise Integration 分离

平台不维护万能 OpenAI/Claude/Gemini/TTS/ASR 协议转换层。CLIProxyAPI、LiteLLM、New API、专用 Speech Gateway 或企业内部推理服务属于 **Upstream Adapter**，承担 Provider 协议兼容和 Provider-specific usage 提取。

`Runtime Relay` 只做身份、策略门禁、资源授权、Route、Credential、透明转发、Correlation 与 request-level Metering。

S0.3 的 **Tool Integration** 只把受信 MCP 工具接入 Enterprise Tool Gateway，并固定为 READ_ONLY/企业共享凭据边界。ERP、CRM、数据库、内部 SaaS/API 的通用接入仍属于 Phase 2 **Enterprise Connector**；Connector 面向 Agent Runtime/Fleet 提供多协议数据与动作能力，不因 S0.3 已支持 MCP Tool Integration 而提前，也不与 Upstream Adapter 合并成模糊的“Adapter”体系。

### 5.8 Secret 默认留在服务端

企业共享 Provider/TTS/ASR/MCP Secret 不下发 Android。Android 持有的是 Enterprise Session；Relay 根据 Runtime Route 注入 Upstream Credential。

用户个人 OAuth 仍可以属于客户端自己的本地能力，不应被强行服务端化。

### 5.9 强制配置依赖自有协议，而不是 Push 必达

MEASIX 不依赖 Firebase/FCM 或任何第三方 Push 保证强制状态。正确性由：

1. 顶层企业 Runtime 开始前的 Managed State preflight；
2. Runtime Relay 对每个 Managed Runtime 请求的 generation barrier；

共同保证。S0 不要求在线事件通道；若后续确有 UX 价值，可增加 SSE/通知作为提示机制，但不得成为 correctness 依赖。

### 5.10 一次 Runtime Interaction 固定一个 Effective Runtime

一次 Android 顶层 `interactionId` 启动后，其 `managedGeneration`、Effective Runtime、工具/资源可见性在该 interaction 内保持一致。若中途发现强制新 generation，应终止当前 interaction、同步后重新开始，不能在同一个 Tool Loop 中静默切换策略。

### 5.11 Metering 与 Quota 分离

- Metering 回答“用了多少”；
- Quota 回答“还能不能继续”。

Usage Ledger 是事实来源；Quota 是运行前资源决策。S0 只完成 Metering/Cost，不建设通用 Quota Engine；S1 对 Agent Space 的 storage/compute 等真实资源实施局部限额，Phase 2 再把跨资源 quota/policy 提升为平台能力。Billing 不反向污染 Runtime 计量模型。

### 5.12 长期术语与 Stable ID 先于实现固定

平台从 Phase 1 开始就固定 terminology/identifier contract，而不是等到多租户或同步阶段再统一：

- Phase 1 当前企业技术边界使用 `deploymentId (dep_*)`；Phase 3 引入 `tenantId (tnt_*)` 后，Deployment 仍保持独立身份，不把 dep 改名成 tenant；
- `userId (usr_*)` 不由 email/username/OIDC subject 派生；外部身份只映射到稳定 User；
- `deviceId (dev_*)` 是 Control Hub 签发的授权对象，Android `installationId (ins_*)` 只是本地安装关联值；
- 客户端逻辑 Model 使用稳定 `modelId (mdl_*)`，Provider/Adapter 的 model name 使用 `upstreamModelKey`，不得混用；
- `providerId`、`modelId`、`upstreamId`、`runtimeRouteId` 分别代表客户端定义、逻辑模型、实际后端和服务端执行路由；
- `managedGeneration/controlRevision/gatewayControlRevision/managedStateRevision` 是版本序列，不是实体 ID；
- Agent Space、Agent Definition/Run、Runtime Hook Definition/Invocation 从进入 Phase 1 的对应阶段起即使用独立 stable ID。

完整术语、前缀和生成规则以 `measix-platform-terminology-and-identifier-contract.md` 为唯一权威。任何新增核心实体必须先注册术语和 ID namespace，再进入下位协议/代码。

## 6. Phase 1 — Runtime Foundation（近期）

**目标：单企业自托管场景形成可实际运行的企业 Agent 基础层。**

### S0 — Foundation Contract / 基础契约

建立：

- Deployment / EnterpriseUser / Device / Session / Principal；
- Control Hub + Admin Console；
- Capability/Policy/Draft/Release/Snapshot；
- Model/TTS/ASR/MCP 第一批受管能力；
- Managed Assistant + Mature Memory Seed + Assistant Starter 第一批 C 类经验；
- Enterprise Update Feed 与 `get_enterprise_updates` 最小 B 类产品能力；
- Runtime Relay；
- Enterprise Tool Gateway、Published Tool Catalog 与 `discover_tools` / `invoke_tool`；
- Upstream Adapter 接入；
- Android Enrollment、Managed State、Effective Runtime；
- Personal/Enterprise ClientRealm 和 Enterprise Portal MVP；
- Publish 后强制同步；
- request-level + semantic Usage / Cost 基础。

S0 内部顺序固定为 `S0.1 Managed Capability Delivery → S0.2 Enterprise Realm & Experience Foundation → S0.3 Enterprise Tool Gateway & Governed Tool Integration → S0.4 Android Managed Runtime Integration → Final RC`。S0 的价值是跑通“身份 → A/B/C 发布 → 受治理工具发现/执行 → Android 企业域生效 → Runtime Plane → Upstream → Usage”闭环，不实现 Agent Space/Agent Runtime/Fleet。

### S1 — Agent Space / 托管空间

对每个企业用户提供真实 Agent Space：独立 disk/runtime、持久存储、quota、lazy provisioning、MCP/WebDAV access surface、compute/storage metering。现有 `weero-agent-space` 作为实现基础，底层保持独立 disk，不能退化为共享目录拼接。

### S2 — Agent Runtime & Remote Delegation / Agent 执行与云端委派

首次落地 Agent Runtime 原语：

```text
AgentDefinition (agt_*)
      ↓
AgentRun (run_*)
```

Android 看到的 `Remote Agent` 是某个 AgentDefinition 以 delegation target 方式暴露：

```text
LOCAL_ASSISTANT | REMOTE_AGENT
```

Phase 1 保持 request-scoped、单层、同步 delegation：Run 可绑定 Agent Space，支持 status/cancel/timeout/artifact/usage；不实现 recursive fan-out、durable scheduler、后台长期驻留或完整 A2A。

为减少进程数量，S2 的 Agent Runtime 初版允许作为 Agent Space 服务内的独立模块实现；只有 Phase 2 durable workload/Agent Fleet 对故障域和调度提出真实要求时再拆成独立服务。

### S3 — Runtime Hook / 运行介入

固定最少 Hook Point：

```text
before_generation
before_tool_call
```

结果：`ALLOW | DENY | INJECT_CONTEXT | REQUIRE_APPROVAL`，并具备 timeout/failureMode。更多 hook point 按真实需求增加，不提前实现通用 workflow engine。

## 7. Phase 2 — Enterprise Platform（中期）

**目标：从“单企业可运行”升级到“单企业可治理、可同步、可持续运营”。**

### 7.1 Identity & Governance

Organization / Group / Membership、OIDC/SSO、按需 SAML/SCIM、RBAC、Device Management、Assignment、Audit/Retention。

### 7.2 User Sync

建立与 Managed Snapshot 独立的用户数据增量同步：Conversation、Message/branch、Attachment、Preference、用户 Agent/Workspace 元数据，以及企业助手记忆、经验贡献与场景执行状态。不得以 ZIP/WebDAV/S3 Backup 代替同步协议。

### 7.3 Enterprise Experience & Engagement

在 S0.2 Managed Assistant/Memory Seed/Assistant Starter 和 Portal MVP 基础上，完整化 Skill、分配、审核、评估与发布；把 Android/Agent Runtime/Fleet 产生的记忆和经验贡献提升为新版企业经验。Enterprise Portal 扩展同步、备份、经验贡献、通知和后续任务触达。

### 7.4 Agent Fleet — 企业持续运行 Agent 集群

Agent Fleet 是企业自有、可持续运行的 Agent Instance 集合，建立在 Phase 1 Agent Runtime 上：

```text
AgentDefinition
      ↓ deploy
AgentFleet
      ├─ AgentInstance
      ├─ Trigger / Schedule / Event
      ├─ Checkpoint / Resume
      ├─ Concurrency / Resource Policy
      └─ Observability / Usage / Audit
```

Agent Fleet 的输入和能力来源不强行抽象成同一种“Data Source”，而是保持不同语义：

- **Enterprise Connector**：ERP、CRM、数据库、内部 API/SaaS 的数据与动作；
- **Agent Space**：文件、workspace、执行工具和持久产物；
- **Remote Agent**：可委派的其他 Agent 能力；
- **User Sync**：在授权范围内提供用户侧上下文；
- **Trigger/Event**：定时、Webhook、业务事件等启动/唤醒机制。

Agent Fleet 必须具有 Agent identity、checkpoint/resume、暂停/恢复、失败重试边界和完整 Usage/Audit；它不是把 Remote Agent Run 简单改成永久循环。

### 7.5 Enterprise Connector

定义企业业务系统接入边界。Connector 可以底层使用 REST、MCP、数据库协议、消息/Event API 等，但对 Agent Runtime 暴露稳定的数据/动作能力和权限边界。S0/Phase 1 不提前实现 Connector framework。

### 7.6 Hosted Runtime 完整化

Agent Space capacity/admin operations、Agent Runtime durable execution、artifact lifecycle、异步任务、更多 Runtime Hook 与审批；是否拆独立 scheduler/queue 由实际并发和可靠性需求决定。

### 7.7 Metering / Quota / Cost 完整化

policy-based quota、allocation/aggregation、cost center、request → interaction → run → agent instance → resource 的完整追踪。

## 8. Phase 3 — Multi-tenant Platform（长期）

目标是一套平台安全承载多个企业 Tenant：Tenant isolation、Tenant-scoped RBAC/SAML/SCIM、Tenant encryption、Billing、Regional placement、HA/autoscaling/noisy-neighbor isolation、多租户调度、A2A/Federation。

Phase 1 不提前 tenant 化每张表和 API；通过稳定 Deployment/User/Principal/Resource/Agent ID 保留演进路径即可。

## 9. Managed Capability 的长期扩展方式

新增 AI 类型默认不产生新的 Platform Plane。

```text
Model
TTS
ASR
MCP
Search
Image
Embedding
Video
Agent Definition（可暴露为 Remote Agent）
Runtime Hook Definition
Agent Space Policy
...
```

应优先扩展：

```text
Managed Definition
  → Client Capability Description
  → Runtime Route / Runtime Service
  → Usage Meter
```

只有当新能力引入新的生命周期、所有权或一致性模型时，才考虑新增独立 Runtime Component。

## 10. 外部互操作策略

MEASIX 优先拥有自身 Android ↔ Cloud 的稳定协议，不在 Phase 1 为了“标准化”提前引入重型外部 Agent 协议。

- Provider/TTS/ASR：通过 Upstream Adapter 兼容行业协议；
- Workspace 文件：WebDAV/MCP 可作为访问面；
- Remote Agent：Phase 1 使用内部 Delegation Contract；
- A2A/Federation：当出现跨系统互操作需求时，以 Adapter 方式接入，不反向主导内部 Runtime 模型。

## 11. 安全与一致性长期边界

1. 企业 Secret 默认不下发客户端；
2. 客户端不能自报任意 `userId` 获得资源；
3. Managed Snapshot 只有完整验证成功才原子切换；
4. Rollback 使用 Republish 形成新 generation，版本号不倒退；
5. Control Hub 与 Runtime Relay / Enterprise Tool Gateway 状态变化必须可 reconcile；
6. Runtime 中已接受的单请求与新策略之间必须有确定边界，不通过中途篡改流量实现“强制”；
7. Usage 允许 UNKNOWN/PARTIAL，但禁止伪造精确值；
8. 企业绑定下无法验证当前 Managed State 时，新企业 Runtime 默认 fail closed；
9. Phase 1 不冒充 MDM：用户主动解除企业绑定或卸载 App 不属于 S0 的设备强管控范围。

## 12. 演进判定标准

每个阶段只有在满足以下条件后才进入下一阶段：

- 当前阶段的权威状态和 ID 模型已经稳定；
- 新阶段不要求推翻上一阶段 Client/Control/Runtime/Metering 边界；
- 新能力能复用 Principal、Managed Capability、Runtime、Usage 这些基础设施；
- 复杂度来自真实需求，而不是为了提前模拟 SaaS 终局。

Phase 1 的成功标准不是“服务端功能很多”，而是 S0–S3 四个闭环都真实可运行；Phase 2 的成功标准是单企业治理和 User Data Plane 成熟；Phase 3 的成功标准才是多租户规模化和商业化。

## 13. 长期稳定术语

以下只列路线图级概念；精确定义和 ID 以术语契约为准。

| 术语 | 稳定语义 |
|---|---|
| Deployment | 一套具体控制/运行配置域；Phase 1 近似一个企业实例 |
| EnterpriseUser | 企业稳定用户身份 |
| Principal | 当前请求/运行代表的统一主体 |
| Control Hub | 控制面权威中心，不进入大流量 Runtime data path |
| Runtime Relay | 身份/策略感知的透明 L7 runtime relay |
| Enterprise Tool Gateway | Enterprise Realm 的受治理工具运行面，只公开固定 discover/invoke surface |
| Managed Capability | 企业发布的 Definition/Policy 集合 |
| Managed Snapshot | 某 `managedGeneration` 的客户端受管快照 |
| Agent Space | 用户级持久隔离 workspace/runtime resource |
| Agent Runtime | 执行 Agent Definition/Run/Instance 的运行原语 |
| Remote Agent | Agent Definition 作为 delegation target 的角色，不是独立 Runtime 类型 |
| Agent Fleet | 企业持续运行、可恢复、可调度的 Agent Instance 集群 |
| Enterprise Connector | ERP/CRM/DB/SaaS 等业务数据/动作接入边界 |
| Runtime Hook | 对固定 Runtime Hook Point 的受控介入 |
| Usage Ledger | append-only 用量事实 |
| User Sync | Phase 2 用户数据增量同步子系统 |
| Enterprise Experience | 企业助手、Skill、记忆、场景与经验贡献持续提炼、评估和再发布的闭环领域 |
| Enterprise Portal | 企业用户工作台，不是 Admin Console，不拥有 Android 本地状态 |
| Upstream Adapter | AI/Speech/MCP Provider 协议兼容边界 |
| managedGeneration | 客户端 Release/Snapshot 单调版本 |
| controlRevision | Runtime Relay 运行控制版本 |
| gatewayControlRevision | Enterprise Tool Gateway 运行控制版本 |

组件正式名称不加 `MEASIX` 前缀；品牌只用于平台全称和文档命名空间。
