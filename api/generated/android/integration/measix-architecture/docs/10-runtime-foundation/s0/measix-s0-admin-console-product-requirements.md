# S0 Admin Console 产品、交互与浏览器边界

> 状态：S0 Product / UX Requirements Authority
> 版本：2026-09-14
> 上位交付契约：`measix-s0-capability-delivery-contract-spec.md`、`measix-s0-enterprise-realm-experience-contract-spec.md`、`measix-s0-enterprise-tool-gateway-contract-spec.md`
> Wire 权威：`measix-s0-control-protocol.md`  
> 测试规格：`measix-s0-admin-console-testing-spec.md`  
> 具体实现：`topabomb/measix-platform-core/docs/admin-console-implementation.md`  
> 文档职责：统一定义 Admin Console 在 S0.1 基线、S0.2 Experience/Enterprise Updates 和 S0.3 Enterprise Tools 扩展中的产品任务、信息架构、交互、浏览器安全/state boundary 和产品 Exit；不维护 Vue 文件树、store/component/composable、npm 依赖或当前实现状态。

## 1. 产品定位与组件边界

Admin Console 是 Control Hub 的浏览器管理面，也是 MEASIX 长期管理平台的第一个可用产品版本，不是临时 CRUD 面板。

S0.1 管理员必须能够在**不编辑 JSON、数据库或 internal API** 的情况下完成：

```text
User / Enrollment
→ Secret + Upstream + Test + Apply
→ Managed Resources + Policy
→ Pricing
→ Validate
→ Review + Client Snapshot Preview
→ Publish / Activation
→ Runtime / Usage / Cost / System diagnostics
```

浏览器边界固定：

```text
Browser Admin Console
        ↓ same-origin public Admin API
Control Hub
        ↓ private control boundaries
Runtime Relay / Enterprise Tool Gateway
```

Admin Console：

- 不直连 Runtime Relay / Enterprise Tool Gateway `/internal/*`；
- 不持有 Relay/Gateway internal URL；
- 不拥有 server validation、Snapshot compilation、Secret persistence、Pricing calculation 或 RuntimeControlState；
- 不把 form/local optimistic state 当作 active server state；
- production 是 static SPA，不增加独立 Node production daemon；
- generated executable Admin contract 是 wire input，不维护平行 DTO。

## 2. 状态模型

UI 必须持续、明确区分：

- **Draft / Candidate**：已保存或本地编辑、尚未生效；
- **Active Runtime State**：受影响 Gateway/Relay 已 ACK 的 operational runtime state；
- **Published Release**：immutable Release + Client Snapshot + managedGeneration。

动作语义固定：

```text
Save Draft/Candidate ≠ Apply ≠ Publish
202 Accepted         ≠ ACTIVE
Validate             ≠ active runtime change
```

长操作必须显示 authoritative Activation lifecycle；刷新页面后能够恢复同一个 operation，不重新创建第二个 semantic command。

## 3. 产品设计原则

### 3.1 Task-first

页面围绕真实管理员任务组织，而不是暴露数据库表、内部 route 或 revision row。

### 3.2 Typed visual authoring

有封闭 vocabulary 的字段使用 select/toggle/checkbox/chip/picker 等 typed control；管理员不手写 enum、comma-separated capability、internal ID 或 raw JSON。

“可视化编辑”是清晰的 typed controls + relationship selection + state feedback，不要求拖拽低代码拓扑。

### 3.3 Progressive disclosure

主表单先展示完成任务必需字段。Timeout、diagnostic identity、advanced transport 等仅在确实存在并需要时进入 Advanced/Details。

首次使用时，Overview 先用管理员能理解的语言说明“是否已有可供 Android 同步的已发布配置”及下一步入口；Generation、Revision、Activation、哈希和数据库结构等诊断标识可查，但不占据主任务说明。列表以名称、用途和“尚未应用／正在使用／有待应用修改”等状态为首要信息，稳定 ID 与修订号留在详情。

创建上游的默认表单只要求名称、端点和认证资料，并明确“创建候选 → 测试 → 应用”的顺序。传输能力、关联模式、用量能力和超时作为可展开的高级连接设置。封闭枚举向人展示本地化名称，提交值仍使用合同原值；任何英文合同值都不能作为面向普通管理员的唯一解释。

资源工作台默认呈现分区选择和正在编辑的内容；完整资源→上游关系表作为可展开的核对视图。上游详情默认回答“连接是否在使用、地址和认证是什么、下一步该做什么”，修订号与传输/用量细节按需展开。已应用配置的再次应用与待应用修改的首次应用必须使用不同的动作说明。

### 3.4 Recoverable operations

Publish、Apply、Revoke 等不能只靠 toast。页面持续展示 operation state、activation identity、阶段、失败原因和恢复动作。

### 3.5 Precise visualization

关系、趋势、diff、timeline 适合可视化，但 stable IDs、revision、hash、Problem code 必须仍可查看/复制。图形不能成为第二份 editable business model。

### 3.6 Security by presentation boundary

- Admin session 使用 secure HttpOnly browser session；mutation 使用 CSRF；
- Secret plaintext 只短暂存在于输入状态，提交后清除，不进入 browser persistent state；
- Usage/request detail 不展示 prompt/body/credential headers；
- UI logic 依赖 stable HTTP status + Problem code，不解析自由文本决定行为；
- retry/recovery 使用同一 command idempotency identity。

### 3.7 与 Android 一致的配置心智

Admin Console 与 Android 管理同一批企业资源时，沿用一致的概念分组和交互语法：设置入口先显示图标、名称、当前摘要和状态，再进入明确的详情分区；Boolean 策略以整行标题、影响说明和尾部 Switch 呈现；Model/MCP 等引用通过带可用状态的 picker 选择。不可用的当前引用必须保留并就近说明原因，不能静默换成第一个可用项。

PC 宽屏使用分区导航 + collection + selected detail；窄屏将同一状态和动作收敛为单列分区选择与详情。两种布局共享同一 Draft、validation 和 publish owner，不维护桌面/移动两套业务状态。Android 的本地用户设置、辅助模型槽位、用户凭据和设备偏好不因此成为 Managed Snapshot 字段；Admin 只呈现平台合同拥有的资源、默认项和准入策略。

## 4. 一级信息架构

S0 一级导航按已进入的 sub-stage 增量出现；S0.1 基线：

```text
Overview
Users
Resources
Upstreams
Releases
Usage
System
```

S0.2 增加 `Enterprise Updates` 一级入口，并在 Resources 内增加 Managed Assistant / Memory Seed / Starter authoring（见 §9.6–9.7）；两条发布路径不能合并成一个保存/生效动作。

S0.3 增加独立一级导航：

```text
Enterprise Tools
```

它不放入 `Resources > MCP`：后者只表示 Direct Managed MCP；Enterprise Tools 表示 Gateway Profile、Integration、受审核 Catalog 和运行状态。

未来可自然扩展 Resources/Users/Usage/System，但 S0.1 不展示 Organization、Groups、RBAC、Billing、Agent Space、Fleet 等空导航或 coming-soon 占位。

## 5. Shell / list / form 通用要求

### Shell

- desktop-first responsive；
- 左侧主导航可折叠；
- 页面顶部有 title/context/primary action；
- deployment/runtime degraded 等高优先级状态全局可见；
- narrow layout 仍保留核心操作，不因固定宽度变得不可用。

### List / collection

适用时支持 search/filter、status filter、pagination、sortable columns、loading/empty/error、detail、stable ID copy、destructive confirmation。

大量实体优先 table/list；不要全部变成卡片墙。

### Form

- label 永远可见；
- required/optional 清楚；
- field-level helper/error 就近；
- server validation 是最终权威；
- dirty state 持续可见；
- stale conflict 不静默覆盖；
- Discard/Cancel 明确恢复最近 authoritative baseline；
- complex resource editor 使用主工作区/detail/inspector，不堆多层 modal。

## 6. Overview

Overview 需要回答：平台是否可用、当前生效哪个 generation/revision、最近什么需要管理员处理。

主视图先给出企业配置当前是否已发布、修改配置和邀请设备的直接入口；详细运行时/数据库诊断可展开或进入 System。未发布时要按 User/Enrollment → Upstream → Resources → Review/Publish 顺序提示，不把“运行时已收敛”误写成已可下发。

至少展示：

```text
Managed runtime status
Active managedGeneration
Desired / applied controlRevision + hash
Relay ready / last seen
Active users/devices
Managed resource counts by kind
Upstream health
Requests/errors/blocked
Usage completeness
Cost status
Usage ingest lag/spool state
Recent activation failures
```

异常优先于装饰性统计。

## 7. Users / Enrollment

Users 至少支持：

```text
Display Name / Username / Status
Devices (name supplied at enrollment, technical identity expandable) / Last Seen
Enable / Disable
Set or reset password where applicable
Generate Enrollment Code
Device list / Revoke Device
Recent usage context where available
```

Enrollment Code：one-time、short-lived、完整值只显示一次。扫码/复制入口输出 Control Protocol §8 的完整 PLATFORM_ENROLLMENT 资料，包含版本、类型、公共平台 origin、code 与 expiresAt；不能只输出裸 code 让客户端猜地址。QR 和复制内容一致，另行显示可核对的平台地址和到期时间。资料不进入日志或持久浏览器存储，隐藏对话框清理内存与二维码。原始 code 可供独立诊断显示，但不得与“复制接入资料”按钮语义混淆。

平台地址取自部署显式配置的唯一公共 origin，不从管理员浏览器地址推断。HTTP/HTTPS、域名、局域网或公网 IP、自定义端口均可正式使用。管理台显示实际配置地址；未配置时说明修改部署配置的操作，禁止生成猜测地址的接入材料。地址合法不等于手机网络可达，不能宣称管理台已经替设备验证连通性。

设备列表显示配置应用状态及报告时间，按 Control Protocol「当前公共入口与设备应用报告」判定。发布页提供到设备状态的入口，明确区分服务端发布成功与客户端已应用；下载、状态查询和最近联系都不能冒充应用回执或当前在线。状态未知时说明尚无当前有效会话的应用报告；已有报告落后于发布时显示待更新。

Restrictive change 显示 Relay enforcement pending，直到真正 enforce；permissive enable 只有 authoritative finalize 后显示 ACTIVE。

## 8. Upstreams / Secrets

### 8.1 Upstream summary

至少能观察：

```text
Name
Status / verification
Base host summary
Transport capabilities
Auth mode summary
Usage capability level
Candidate revision
Active revision
Last Test / Apply result
```

candidate != active 必须明确可见。

### 8.2 Typed editor

Required configuration：

```text
Name
Base URL
Transport Capabilities
Authentication
  NONE
  BEARER
  STATIC_HEADER
  BASIC
Correlation Mode
Usage Capability Level
Timeout Defaults
```

Auth mode 只展示该模式需要的字段。SecretRef 使用 human-readable metadata/picker/create/replace workflow，不要求用户手写 `sec_*`。

### 8.3 Workflow

```text
Edit candidate
→ Save
→ Test Connection
→ review structured result
→ Apply
→ wait authoritative Activation
→ Active revision updated
```

Connection Test 与 Qualification 必须明确区分；Test PASS 不等于 profile VERIFIED，也不自动 Apply。

### 8.4 Secret UX

Secret plaintext：

- create/replace 后不可回显；
- submit 后输入清除；
- 不进 localStorage/browser persistence；
- normal UI 只显示 name/id/version metadata。
- 创建后及页面重载后均能从仅含元数据的分页列表中选择已有 Secret；选择最新版本不会自动保存或 Apply Upstream。取消创建上游后，已创建 Secret 仍可重新选择，不得成为无法绑定的孤立凭据。

## 9. Resources / Policy

固定 tabs：

```text
Models
TTS
ASR
MCP
Policy
```

每个 capability tab 同时具有：collection、Add、selected editor/detail、binding/runtime summary、validation state。

桌面优先 collection + editor/detail；narrow layout 转单列，但不得丢 Save/Validate/Enabled/Binding 等核心操作。

共享 resource editor 至少表达：

```text
Identity
Capability/Profile
Execution: Upstream + Runtime Path + transport summary
Enabled
Validation / binding state
Stable logical ID as secondary identity
```

Upstream 使用 typed picker，并显示 human-readable name + active/degraded/disabled/verification 状态。`runtimeRouteId` 不作为正常 authoring field。

### 9.1 Model

S0.1 required profile：`OPENAI_CHAT_COMPLETIONS`。

至少编辑：

- Provider picker；
- Display Name；
- Upstream Model Key，与 logical `mdl_*` 明确区分；
- TEXT/IMAGE input modalities；
- TEXT output summary；
- TOOL/REASONING capability booleans；
- Upstream + runtimePath + Enabled。

不暴露 arbitrary custom body/header/provider override。

### 9.2 TTS

按 Control Protocol §10.5 提供 OpenAI Speech、Gemini TTS、MiMo TTS、设备系统朗读四种显式选择。

共同编辑显示名称、语音服务、Enabled。云端服务编辑模型、音色、上游与接口路径；MiMo 音色设计改为必填描述并移除预置音色。设备系统朗读只编辑语速、音调，不显示上游、密钥或请求路径。切换服务须明确提示参数重置并清除不适用字段。输出摘要按所选服务说明 MP3 或 PCM，不展示无关 codec matrix。发布预览显示服务类型及实际语音设置；企业下发系统朗读可作为默认资源，不能与个人 TTS 准入开关混淆。

无权威 runtime Test API 时，不做前端伪试听成功。

### 9.3 ASR

支持 OpenAI multipart 文件转写、DashScope HTTP JSON 文件转写、OpenAI 实时识别和 DashScope 实时识别；协议及字段约束以 Control Protocol §10.6 为准。

共同编辑名称、模型、可选语言、上游、运行路径和启用状态。协议选择明确区分 multipart、JSON 音频 Data URI 与实时录音；切换协议时使用对应默认值，并清除不适用参数、同步路由方法和传输类型。

实时识别的采样率、VAD 和静音时长放入默认折叠的录音参数区，OpenAI 另提供前缀缓冲时长及可选提示词。文件转写不显示这些字段。客户端预览应能确认所选协议和录音参数。

### 9.4 Direct Managed MCP

Profile：`MCP_STREAMABLE_HTTP`。

Auth Ownership 只允许：

```text
ENTERPRISE_MANAGED
NONE
```

ENTERPRISE_MANAGED 明确说明 credential 在 server-side Upstream/Secret；NONE 不制造无意义 Secret requirement。S0.1 不展示 USER_MANAGED OAuth/custom-header DSL。

新建 Direct MCP 上游绑定默认允许 POST、GET、DELETE，以覆盖 Streamable HTTP 请求、可选服务器流和会话关闭。服务不实现可选方法时，其 405 响应由 Relay 透传；界面不应默认在路由层提前拒绝这些标准方法。显式编辑过的方法限制保留管理员选择。

### 9.5 Policy

Local coexistence 使用独立、语义明确的 controls：

```text
Allow Local Models
Allow Local TTS
Allow Local ASR
Allow Local MCP
Allow Local Assistants
```

Default Model/TTS/ASR/Assistant 使用对应 resource picker，只允许有效且 enabled 的同 kind resource。管理员可明确指定首次企业会话的默认助手；无效 default 必须形成可导航 validation error。Preview/Review 展示 Android 实际收到的默认助手，清空默认助手不影响空间页进入，客户端应提供助手选择入口。

### 9.6 Managed Assistant / Memory Seed / Starter（S0.2）

管理员通过 typed editor 维护 Assistant 名称、说明、system prompt、enabled Model 引用、可选 Direct MCP 引用、只读交付的 memory seed 条目与 enabled。Starter 归属一个 Assistant，维护 title/prompt/description/sortOrder/enabled；不要求手写平台 ID、JSON 或 Android Local schema。

Save 只改 Managed Draft；Validate 展示缺失/disabled 引用；Preview/Review 显示最终 Assistant/Seed/Starter projection 和 diff；Publish 随 Snapshot v4+ 原子生效。编辑 seed 不修改用户运行记忆，Starter 只预填且不 auto-send。精确字段和生命周期服从 Realm/Experience Contract。

### 9.7 Enterprise Updates（S0.2）

独立列表/详情/编辑面提供 title、content、contentFormat、category、severity，以及 Draft→Publish→Withdraw。状态、发布时间、Feed revision 和失败恢复可见；只读 Markdown 预览遵守 Realm/Experience Contract 的安全子集。

列表和详情以实际排版后的公告正文、标题、状态和本地时间为主；Markdown 源码标记、Feed revision 与稳定 ID 只用于技术核对。发布和撤回确认使用公告标题标识对象，不要求管理员辨认 ID。

发布/撤回动态仅更新 Feed，不改 Managed Draft/Release/generation。PUBLISHED 内容不能当 Draft 原地编辑；用户明确看到当前可见内容与未发布编辑工作区的区别。不引入回执、评论、定时发布或分组投递。

## 10. Enterprise Tools（S0.3）

一级页面必须提供：

```text
Gateway Guidance
Integrations
Tool Catalog
Catalog Drift
Gateway Status
```

### Gateway Guidance

管理员可编辑 Discover Guidance、Invoke Guidance，并从 Published Candidate selection 中选择 Highlighted Tools。页面必须同时显示：

- Gateway 是否纳入该 Release，以及 client enablement policy：强制开启 / 默认开启且允许企业用户关闭；
- 平台固定合同（只读）与企业 guidance segment 的边界；
- canonical `discover_tools` / `invoke_tool` 最终 description/schema preview；
- highlighted tools 自动渲染结果与悬空/disabled validation；
- description length/token estimate（只作预算，不伪造 Provider cache hit）；
- 当前 Draft 与 Published surface/generation diff。

企业不能改两个工具名称、平台安全合同、input schema 或固定顺序，也不能分别开关两个工具。关闭 Gateway 表示从下一 Release 省略整个 Gateway resource；client policy 只在 Gateway 已发布时决定 Android 用户能否成对关闭。

### Integrations

每个 Tool Integration 至少显示 stable ID、display name、MCP Streamable HTTP endpoint summary、credential reference 状态、candidate/published drift、last test/refresh、enabled/health。Secret plaintext 提交后立即清除，不回显。

动作必须分开：

```text
Test Integration
Refresh Catalog
Save Candidate review
Validate
Preview
Publish
```

不能用一个“保存并生效”混淆。Refresh 只形成 Candidate/diff，不改变 Published Catalog/generation。

### Tool Catalog / Drift

每个工具至少显示：source name、stable `gatewayToolId`、agent-facing name、source/agent description、aliases、input/output schema、enabled、risk、candidate/published state、schema hash/diff、integration source。

S0.3 只允许选择并发布 `READ_ONLY`。管理员可修改 agent-facing name/description/aliases/enabled，但参数 required/type/validation constraint 只读；MCP annotation 必须标为 untrusted source evidence，不能自动授予 risk/permission。相同来源的 Direct/Gateway 双路径冲突必须可导航并阻止 Publish。

### Gateway Status

显示 desired/applied gatewayControlRevision/hash、Gateway ready/build/last seen、active/dormant catalog hash、integration/downstream health、drift、最近 Activation/reconciliation。页面不把 supervisor `active` 冒充 Gateway ready，也不承担通用日志浏览器职责；可提供安全诊断提示/相关 event correlation，但不展示 internal URL、resolved credential、toolRef claims、原始日志或 tool arguments/results。

## 11. Configuration Relationship View

提供轻量、只读、可操作的：

```text
Resource → Upstream → candidate/active state → runtime path/transport
```

默认优先紧凑 table/list；只有实体/关联复杂度确实需要时才用 node-edge graph。

必须支持 kind filter、missing binding、disabled/unverified/degraded/candidate!=active 状态，以及 click-through 到对应 Resource Execution 或 Upstream detail。

## 12. Validate / Review / Client Snapshot Preview

### Validation

- Errors block Publish；
- Warnings require explicit review；
- UI 使用 stable code + severity；
- issue 可定位到 tab/resource/field。

### Review

Review 的 Published→Candidate 差异由 Hub 将当前已保存 Draft 与最新不可变 Release 比较后返回；浏览器不得把保存后的本地 baseline 当成 Published，也不得自行重建权威差异。没有 Release 时明确显示“无已发布基线”，并把当前候选计为新增内容。

当已有 Release 且权威差异为零时，Review 应明确说明草稿与当前发布相同，不提供普通 Publish 动作；重新应用历史 Release 使用 Releases 的明确操作，避免管理员因重复点击产生无意义的新发布。投影哈希等诊断标识留在可展开的技术信息中。

Review 默认只列实际发生变更的资源类别和运行时影响，能直接打开同一份 canonical Client Snapshot Preview 核对 Android 最终可见内容，然后返回审查继续发布；不以全是零的表格或单个变更总数替代内容核对。

正式审查面至少展示：

```text
Added / Changed / Removed Resources
Policy changes
Runtime routing impact
Warnings
```

复杂对象允许展开 before/after detail，不用 raw internal DTO 代替产品 diff。

### Client Snapshot Preview

必须来自 Hub canonical compiler，且与真正 Stage/Publish 使用同源 projection。

清楚区分：

```text
Client receives:
  current Snapshot v4: Providers / Models / TTS / ASR / Direct MCP / Policy
                       Assistants / Memory Seed / Starters
  future Snapshot v5: Gateway logical resource / surfaceHash

Client never receives:
  Upstream/base URL / Secret / runtimeRouteId / Binding / Pricing
  Tool Integration / Catalog / downstream URL / toolRef / Gateway internal state
```

Preview 无 Release/generation/control side effect，浏览器不自行重建 Snapshot。
同一已保存 Draft 重复预览时，`projectionHash` 必须保持稳定，方便管理员识别内容是否变化；它不是包含正式 Release 元数据的 `snapshotHash`。

## 13. Publish / Releases

S0：`Publish = activate + enforce`。

Publish progress 至少表达 validating/staging/applying-gateway/applying-relay/finalizing/active/failed-degraded 这些真实阶段，不由浏览器模拟成功。目标含 Gateway 时，即使 Catalog 字节不变也须显示新 generation 映射的 apply；移除 Gateway 按 Protocol 显示 route 移除与后续 cleanup，不能用“无 Gateway 内容变化”跳过必要控制操作。refresh 恢复同一 Activation。

Releases 至少显示 generation、release identity、status、published metadata、Snapshot hash、source Draft revision、diff summary、activation history。Timeline 用于历史，不创造第二份状态真源。

发布历史列表以“第 N 版”、本地发布时间、变更摘要和是否仍活跃为主；打开详情后才显示激活时间线、重新发布动作及可展开的 release ID、快照哈希、草稿修订号和发布者 ID。旧版本状态要表达“已被新版本替代”，避免直接露出状态码。

## 14. Pricing / Usage

### Pricing

至少可维护：scope(resource/upstream)、meter、unit size、unit price、currency、effective time/revision。

必须区分 Known / Partial / Unknown cost；没有可靠 semantic meter 时不得把 0 当成“无成本”。

### Usage

Summary 至少覆盖 Model/TTS/ASR/Direct MCP/Gateway 五条 Managed runtime path，以及 requests/errors/blocked、可靠 semantic meters、usage completeness、cost completeness。Gateway request fact 与 resolved tool execution fact 必须可 correlation，不能 double count。

Filters 可组合：

```text
Time
User
Resource
Resource Kind
Upstream
Status
Usage Completeness
```

Request Detail 只展示允许的 correlation/generation/resource/upstream/status/duration/meter/cost 诊断信息，不展示 prompt/body/Secret/credential header。

## 15. System / diagnostics

System 是只读诊断面优先，至少展示：

```text
Hub build/version + DB/migration health
Relay ready/version/last seen
Gateway ready/version/last seen
Managed runtime status / active generation
managedStateRevision
Desired/applied controlRevision + bundle hash
Desired/applied gatewayControlRevision + bundle hash
Current/last Activation
Metering ingest lag/spool
Semantic orphan/unknown
Upstream health summary
Tool Integration / Catalog drift summary
Last reconciliation
```

未来危险修复动作必须独立设计确认/审计语义，不因为“System 页面存在”就默认允许 arbitrary mutation。

## 16. 可视化 / accessibility / responsive

- 状态颜色必须配文本/icon，不依赖颜色 alone；
- collection/editor header 都能识别 saved/dirty/valid/warning/invalid/missing-binding/degraded/disabled；
- chart 有时间范围/单位/precise hover，unknown 不画成 0；
- diff 用于配置变化，timeline 用于 activation/reconciliation 历史，relationship view 用于 resource→upstream；
- visible focus、label/error association、keyboard-friendly main workflows；
- modal 只用于短确认/局部输入；
- 320px–desktop 不出现无法操作的固定宽度表单。

## 17. 产品 Exit

只有同时满足以下条件，Admin 才能进入 S0.1 system freeze：

1. clean environment 仅通过 public Admin UI 完成 User/Enrollment、Secret/Upstream、Resources/Policy、Pricing、Validate、Preview、Publish；
2. Draft/Candidate、Active Runtime、Published Release 三类状态表达正确；
3. required Model/TTS/ASR/MCP/Policy 全部有 typed visual editor，不需要 raw JSON/internal ID；
4. resource collection/editor 能看到 identity、capability、binding、runtime、enabled、validation；
5. relationship view 能定位 resource→upstream/runtime 错误；
6. Snapshot Preview 与 canonical compiler 同源且 client-safe；
7. Apply/Publish 可 refresh recovery、不重复 command；
8. Usage/Pricing 按四 Resource Kind 可观察，UNKNOWN/PARTIAL 不伪造；
9. Secret/internal route 不进入 browser persistent state；
10. production SPA + real browser + real Hub/Relay 的 T4.1 golden path Green。

S0.2 Admin 扩展进入 Realm/Experience Freeze 前，必须通过真实浏览器完成 §9.6–9.7 两条独立发布路径、Snapshot v4 Preview 与 Feed 可见性验证，且不改用户记忆/不 auto-send/不以 Feed 更新推进 generation；required evidence 由 Realm/Experience Testing Spec 的 `ERX-C-*`、`ERX-UPD-*` 定义。

S0.3 Admin 扩展只有在以下条件同时成立时才能进入 Gateway Freeze：

1. 仅通过 public Admin UI 完成 Integration→Test→Refresh Candidate→Review READ_ONLY→Guidance/Highlighted→Validate→Preview→Publish；
2. Direct MCP 与 Enterprise Tools 分属明确入口，同源双路径被阻止；
3. 两个 Meta Tool 最终 description/schema/surfaceHash 与 canonical compiler/Gateway `tools/list` 一致；
4. Candidate/Published/drift/generation 和各个独立动作不混淆；
5. Catalog schema/description/alias/risk/source evidence 可审查，schema constraint 不可编辑；
6. Publish 展示 Gateway-first/Relay-second authoritative Activation，失败/刷新可恢复；
7. Gateway status/reconciliation/drift 可诊断且不泄漏 private material；
8. production SPA + real browser + real Hub/Gateway/Relay/Test Client 的 T4.3 golden path Green。
