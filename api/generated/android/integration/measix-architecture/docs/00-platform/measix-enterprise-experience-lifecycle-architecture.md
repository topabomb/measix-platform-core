# Enterprise Experience 生命周期与进化闭环架构

> 状态：Platform Domain Architecture Baseline / 平台领域架构基线
> 版本：2026-08-31
> 上位文档：`measix-agent-platform-roadmap.md`
> 术语/ID 权威：`measix-platform-terminology-and-identifier-contract.md`
> 文档职责：定义企业资源、能力与经验的关系，以及企业经验从发布、运行、记忆回流、提炼到再发布的长期闭环；不定义具体 REST endpoint、数据库表、同步 wire schema 或 UI 组件。

## 1. 平台目标

MEASIX 的企业价值不只是“企业能向 Android 下发什么”，而是：

> 企业把资源、能力和已有经验交给 Agent，Agent 在真实业务中产生新记忆和新方法，企业再将其提炼为新一版可复用经验。

这个闭环服务 Android Client，也必须能在后续承接 Agent Runtime 和 Agent Fleet 产生的经验。

## 2. 完整进化闭环

```text
Enterprise authoring / curation
  ├─ Runtime Resources (A)
  ├─ Enterprise Capabilities (B)
  └─ Experience Assets (C)
          ↓ validate / immutable Managed Release
Managed Delivery / Assignment
          ↓
Android Enterprise Realm
  ├─ managed assistant / skill / starter projection
  ├─ conversation / tool / task execution
  └─ enterprise assistant memory evolves
          ↓ User Sync
Enterprise Assistant Memory
          ↓ extract / aggregate
Experience Contribution
          ↓ review / evaluate / curate / promote
New or updated Experience Definition
          ↓ new Managed Release / managedGeneration
Android / Agent Runtime / Agent Fleet reuse
```

闭环中任何一个运行事实都不直接改写已发布 Definition。经验只能通过提炼和新 Managed Release 进入下一轮交付。

## 3. 企业交付 A/B/C 产品分类

### 3.1 A — Runtime Resource / 运行资源

回答“用什么运行”：Provider/Model、TTS、ASR，以及后续 Image/Embedding/Video、企业统一费用资源和 Agent Space 的 compute/storage。

### 3.2 B — Enterprise Capability / 企业能力

回答“能查询、执行和委派什么”：MCP、Search、本地工具、Enterprise Tool Gateway，以及知识库查询、业务动作、Enterprise Connector、Remote Agent 和 Agent Space 能力。分类不自动授予企业域准入权限。

### 3.3 C — Experience Asset / 企业经验资产

回答“怎样把事情做好”：Assistant、Skill、提示词/注入、快捷提示词、Memory Seed、Starter，以及工具编排方法、检查清单和场景模板。Memory Seed 是只读配置定义；运行产生的聊天、记忆与文件属于运行数据。

### 3.4 Policy 是横切治理层

Policy 决定 A/B/C 的分配、可见性、默认/强制语义、本地扩展、工具权限、记忆同步、保留与共享范围。Policy 不作为第四种业务内容。

`Managed Capability` 仍是承载这些 Definition/Policy 的平台技术总称。

主题、显示和输入等用户偏好单独分类，不归入 A/B/C 或 Policy。A/B/C 同样适用于用户定义，不能把分类本身理解为仅有企业内容。

## 4. ClientRealm 与数据边界

Android 长期存在两个顶层 Realm：

```text
PERSONAL
  Built-in + User Configuration + Personal Preferences

ENTERPRISE(deploymentId)
  Enterprise Managed + policy-allowed User Configuration + Enterprise Preferences
```

固定语义：

- 不可移除的安全/导航等 Host 功能不是 A/B/C 内容；不可借 Built-in 名义绕过企业内容策略。S0.3 Gateway pair 是受发布与 clientEnablementPolicy 管理的 Enterprise Managed 内置能力，而非 Personal 工具；

- Personal Realm 不显示、不注册、不解析、不执行企业内容；
- Enterprise Realm 获准使用用户资源时直接引用其唯一原定义，连接时使用用户原凭据；不创建独立企业资源副本，不上传凭据，也不注入企业 Session；
- Conversation、运行 Memory、Attachment、Workspace 和运行产物必须保留域与主体归属；Assistant 等配置定义的复用不共享这些运行数据；
- 切换 Realm 不改写已有对象归属，也不在运行中 interaction/tool loop 里切换 Effective Runtime；
- 企业按已定义的策略控制项决定用户配置准入；当前五项全部关闭只禁止对应五类用户配置，不代表所有 A/B/C 都“仅企业内容”。策略改变企业域可用性，不全局禁用、删除或改写用户原定义。

### 4.1 用户配置准入

| Policy 字段 | 允许参与企业域解析的用户配置 |
|---|---|
| `allowLocalProviders` | 用户 Provider 及其 Model |
| `allowLocalTts` | 用户 TTS |
| `allowLocalAsr` | 用户 ASR |
| `allowLocalMcp` | 用户 MCP |
| `allowLocalAssistants` | 用户 Assistant |

`true` 允许引用已有定义，`false` 仅禁止企业域使用，个人域保持可用。允许用户助手不连带授权其引用资源；各资源仍逐项受对应策略约束。被禁止、缺失或不可用的引用必须有明确原因并阻止不合规执行，不能静默换资源或绕过策略。企业助手的模型、提示词和工具绑定继续受管、不可修改。

本期已确认的控制清单之外准入规则：用户 Search、Skill、提示词注入、QuickMessage、本地工具、Workspace 配置可在企业域选择和使用，保留既有授权、审批及系统权限。这是独立产品决策，不由五个 Boolean 的值推导；后续增加控制项必须明确默认值和版本兼容，不能默认为未知类别授予系统权限，也不能按“尚无开关”禁用上述已允许的配置。搜索是否携带 API Key 不改变其 B 类归属。

子助手仍是 Assistant：用户主/子助手均受 `allowLocalAssistants` 约束，并叠加原有主从引用和授权关系；子运行继承父运行的域/主体。Search、Skill、注入、本地工具或委派实际使用 Provider/Model、TTS、ASR、MCP、用户助手时，仍逐项复验相应控制项，不能以入口不受控绕过底层资源准入。

用户助手的企业域使用选择保存在本域偏好，不复制或改写原定义。显式引用失效时保留引用并显示原因，阻止发送，提供本域重选允许资源的入口；没有显式引用才解析本域默认，不按名称或首项替换。企业助手固定引用由企业修正，引用不闭合的下发候选整包拒绝。相同稳定 ID 的名称/参数更新仅影响后续执行；停用/删除使选择不可用，在途执行不得静默换资源，后续请求仍受已知撤销和版本屏障约束。

受管助手未由企业固定且客户端允许调整的使用参数、扩展选择及显示设置可保存在本域偏好；初值来自普通助手的产品默认，不复制当前个人助手或填出第二份受管定义。偏好不得覆盖企业固定的身份/模型/提示词/MCP 引用，不得以会话提示词覆盖、请求 Header/Body 或扩展参数改写已解析路由、认证或资源准入。用户选择的额外工具和子助手仍叠加各自授权；企业定义的字段不能由“本域设置”间接变成可编辑。

Workspace 配置可复用不等于其目录文件自动获得物理域隔离。用户必须显式选择已有共享 Workspace，界面说明共享文件的影响，保留目录权限和访问约束；域内聊天、运行记忆及非共享产物仍按域/主体保存。企业导出、通知正文和退出后数据保留的完整治理策略仍需独立合同。新策略默认与兼容以 Control Protocol §10.10.1 为准。

### 4.2 Direct Managed MCP 与 Gateway

企业发布为启用的 Direct Managed MCP 在企业域自动启用，用户不能关闭、编辑或删除；它不受 `allowLocalMcp` 影响。UI 显示企业来源与强制启用状态。服务启用与助手工具绑定分离：仍按助手 `mcpServerIds` 暴露工具，不给所有助手注册全部工具。

Gateway 的 `REQUIRED` 和 `USER_CONTROLLABLE_DEFAULT_ON` 保持独立合同；允许调整的偏好按企业/Gateway 保存，不能据此允许用户关闭 Direct Managed MCP。

### 4.3 配置归属与运行捕获

用户配置、用户偏好、企业下发配置的定义见术语合同 §4.11。企业偏好只保存选择、允许调整的使用参数和开关，不是 `enterprise_local` 资源副本。身份、Session/凭据、同步、运行数据、缓存和恢复状态继续由各自 owner 管理。

App 从这些配置 owner 派生唯一只读生效快照，表达当前域、企业、适用版本、资源/助手/工具、实际选择与参数、来源、可选择/可修改/强制启用/不可用原因及显示操作偏好。写入回到对应 owner 后重新派生；写入与执行边界重新校验，不能只靠 UI 禁用。一次运行开始捕获配置和域/主体，后续切域与更新不能静默改变该运行。

## 5. Experience Definition 与运行状态

### 5.1 ManagedAssistantDefinition

企业下发、由 Android 本地 Runtime 执行的助手定义，可引用：

```text
instructions
model/resource policy
enterprise capability refs
skill refs
initial experience / memory seed
memory policy
starter refs
```

它与 Agent Runtime 执行的 `AgentDefinition` 保持独立身份。后续可以共享子定义或通过显式 execution placement 适配，但不得因名称相似隐式合并生命周期。

### 5.2 ManagedSkillDefinition

Skill 是可发现、可引用、可评估的经验模块。Android/Agent 按以下层级渐进披露：

```text
catalog metadata
  → primary instructions
      → references / scripts / assets on demand
```

发布时必须固定版本内容、依赖、兼容性、权限声明和完整性摘要。

### 5.3 AssistantStarterDefinition

S0.2 Starter 是 Managed Assistant 的“常用入口提示词”，包含 assistant reference、title、prompt、description、sort order 和 enabled。Starter Definition 通过 Managed Release 发布；Android 点击后只预填提示词并等待用户确认。表单、附件、任务和 workflow 起点不属于该 Definition；面向某用户的定向投递仍是未来 EnterpriseActivity。

### 5.4 EnterpriseAssistantMemory

企业助手在用户真实工作中产生的可持续更新记忆。它与助手定义关联，但不是定义的一部分：

```text
ManagedAssistantDefinition   immutable per release
EnterpriseAssistantMemory   mutable user data
```

定义升级不覆盖已有记忆；记忆同步不原地修改已发布定义。

### 5.5 ExperienceContribution

对一条或多条记忆、会话、工具结果、AgentRun/场景结果的可复用提炼。Contribution 是待治理对象，不是已发布 Experience Asset。

### 5.6 EnterpriseUpdate

EnterpriseUpdate 是 Portal 与企业动态查询能力共用的动态内容 authority。它不进入 Managed Snapshot，不随 Assistant/Starter Release 版本化；Feed revision/ETag 与 `managedGeneration` 分离。

## 6. 向下交付：Managed Experience Delivery

企业经验定义与 A/B 一样进入 Draft 、Validate、Review、Managed Release 和 Managed Snapshot：

```text
Experience Draft
→ validate references / policy / compatibility / package integrity
→ immutable Managed Release
→ monotonic managedGeneration
→ assignment-aware client projection
→ Android atomic apply
```

固定边界：

- Snapshot 只承载 Definition/Policy，不承载企业助手运行记忆、会话和 Contribution；
- 经验定义只在完整验证后与当代 Snapshot 原子切换；
- 同一 interaction 固定当代 Effective Runtime 和 Experience Definition 集；
- 企业撤回/更新 Definition 不删除历史 Conversation/Memory，保留策略由 User Data Governance 决定。

## 7. 向上回流：Memory 与 Experience Sync

企业助手记忆原本就是企业经验进化的输入。长期必须支持回传，但传输与发布语义分离：

```text
Android local durable commit
→ User Sync change capture
→ server durable acceptance / dedup / ordering
→ memory materialization
→ contribution extraction
```

同步记忆必须保留：

- Deployment/User/Assistant 归属；
- source Conversation/Message/Tool/Run/Starter/Task provenance；
- 内容类型、敏感级别、保留与共享范围；
- 产生时的 Assistant/Skill/Capability/managedGeneration；
- 源端 stable ID、revision 和幂等/冲突所需元数据。

Android 完成本地持久化是事实成立的前提；同步失败不能把已成功的本地运行记录为失败，应进入可重试同步状态。

## 8. 企业提炼、评估与提升

回传记忆可直接用于原用户的跨设备连续性；只有经过提炼才能扩大共享范围。

```text
COLLECTED MEMORY
→ EXPERIENCE CANDIDATE / CONTRIBUTION
→ REVIEW
→ EVALUATION
→ APPROVAL
→ PROMOTION TO DEFINITION
→ NEW MANAGED RELEASE
```

提升结果可以是：

- ManagedAssistantDefinition 新版；
- ManagedSkillDefinition 新增/新版；
- AssistantStarterDefinition；
- 共享记忆/知识种子；
- 工具组合、检查清单、失败条件或评估样例；
- 对 AgentDefinition、Runtime Hook 或企业治理策略的修改。

一个 Contribution 可被拒绝、合并、拆分或被多个 Definition 引用；必须保留 Contribution 到新 Release 之间的 provenance。

## 9. 用户与管理体验边界

### 9.1 Android Client

- 企业域内执行 Managed Assistant/Capability，并展示 Assistant Starter；
- 在本地先持久化 Conversation、Memory 和产物；
- 显示经验来源、锁定/可扩展状态和同步状态；
- 把同步、贡献、场景与任务流程导向 Enterprise Portal。

### 9.2 Enterprise Portal

S0.2 面向企业用户交付 Native Host、企业接入/会话/同步状态、操作按钮和 Enterprise Updates 列表/详情。后续再增加同步、备份、助手/Skill 目录、记忆/贡献、通知、任务和 Agent Space/Remote Agent 入口。

Portal 来自独立企业前端项目，通过受限原生 capability bridge 使用扫码、相机、照片、文件、麦克风等手机能力；它不直接写 Android Settings/Room，也不持有无限制的原生接口。

### 9.3 Admin Console

面向管理员，承载企业经验的 authoring、assignment、review、evaluation、promotion、release、rollout、withdraw 和 provenance/audit。Admin Console 与 Enterprise Portal 是两个用户对象和任务不同的产品。

## 10. Enterprise Engagement

企业主动触达是经验分发与任务运营的一部分，包含：

```text
Notification
Starter delivery
Conversation start request
Task assignment
Run/result reminder
Experience review request
```

- Starter Definition 属于 Experience Asset；
- 向某个用户的投递属于 EnterpriseActivity；
- 用户启动后产生的 Conversation/Task state 属于 User Data；
- 执行产生的 Memory/Contribution 重新进入经验闭环。

EnterpriseActivity 不得向已有个人会话静默插入消息，不得绕过用户或运行门禁直接触发有副作用的工具。Push/SSE/系统通知是到达优化，不替代 Managed State preflight 和 generation barrier。

## 11. 企业接入与 Portal Session

Phase 1 Android 通过扫码或粘贴一次性 Enrollment 资料建立 Device、Binding 和 Enterprise Session，不要求首期建设完整登录/注销/多组织账号体系。

产品上会话采用服务端权威七天滚动闲置时限。Enrollment 初始化 expiry；只有成功的 authenticated Refresh 原子轮换 Refresh Credential 并续期，普通 API/Runtime/同步/Portal load 不单独续期，也不使用后台 heartbeat 保活。内部使用短期 Access Token + 受保护 Refresh Credential，但长期 Session identity、Device revoke 和解除企业接入语义保持稳定。

Enterprise Portal 使用由 Android/平台为当次 Portal 会话建立的受限 Web Session；不将 Android Refresh Credential 暴露给 WebView JavaScript。

## 12. 长期不变式

1. Personal Realm 不消费任何 Enterprise Managed 内容。
2. Enterprise Realm 按 §4 Policy 直接引用用户唯一配置及其凭据；运行数据按域与主体隔离，不能把定义复用解释为共享个人数据。
3. Managed Definition 与企业助手记忆永久分离。
4. Managed Delivery 与 User Sync 共同服务闭环，但不共用版本/冲突模型。
5. 未审核 Contribution 不是可下发经验。
6. 正式经验只通过新的不可变 Managed Release 交付，不直接改写已发布 Snapshot。
7. 记忆/经验必须保留产生时的 Principal、Definition、Capability 和 managedGeneration provenance。
8. Enterprise Portal 是用户产品界面，不是 Android 本地状态或 Control Hub 权威的替代者。
9. Notification/Push 不承担 Managed Runtime correctness。
10. Android 仍是完整 Local-first Runtime；经验闭环不将它降级为云端薄客户端。

## 13. 阶段映射

### 当前确定的阶段关系

- S0.1 建立 Identity、Enrollment、Session、当前 Managed Release/Snapshot、A 资源和 Direct Managed MCP 基线；
- S0.2 建立 ClientRealm、Snapshot v4、Managed Assistant + Memory Seed + Assistant Starter、Enterprise Updates capability/Feed 和 Portal MVP；
- S0.3 建立 Enterprise Tool Gateway、受审核 Catalog、`discover_tools` / `invoke_tool` 与 Snapshot v5 服务端闭环；
- S0.4 完成 Android Model/TTS/HTTP-ASR/Direct MCP/Gateway runtime integration、failure/concurrency 和最终设备闭环；
- S1 Agent Space、S2 Agent Runtime、S3 Runtime Hook 保持现有独立语义；
- Phase 2 User Sync 承载企业助手记忆和 Experience Contribution 回流；
- Phase 2 Identity/Governance 承载分组分配、审核、保留、共享与 Audit；
- Agent Runtime/Fleet 后续与 Android 一样成为经验生产者。

S0.2 不交付 Skill、Memory User Sync、Experience Contribution 提炼、Push/Activity 或任务编排。它们保持后续阶段；S0.2 只建立不会阻碍进化闭环的对象、ownership 和产品入口。
