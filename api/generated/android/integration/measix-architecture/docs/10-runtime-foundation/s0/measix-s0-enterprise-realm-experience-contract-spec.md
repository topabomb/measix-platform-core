# S0.2 — Enterprise Realm & Experience Foundation Contract

> 状态：S0.2 Foundation Contract / 企业域与经验基础基线
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`
> 生命周期权威：`../../00-platform/measix-enterprise-experience-lifecycle-architecture.md`
> 术语/ID 权威：`../../00-platform/measix-platform-terminology-and-identifier-contract.md`
> Wire 权威：`measix-s0-control-protocol.md`
> 产品要求：`measix-s0-enterprise-portal-product-requirements.md`
> 测试权威：`measix-s0-enterprise-realm-experience-testing-spec.md`
> 下游阶段：`measix-s0-enterprise-tool-gateway-contract-spec.md`（S0.3）、`measix-s0-android-integration-contract-spec.md`（S0.4）
> 文档职责：固定 S0.2 的 Personal/Enterprise ClientRealm、企业接入、A/B/C 体验基础、企业动态 Feed、Portal MVP、Android 投影和 Exit；不定义 Gateway 工具合同、Kotlin class/file、数据库 DDL 或前端组件实现。

## 1. 阶段目标

S0.2 在 S0.1 A 资源与基础 B/MCP 交付骨架之上，完成第一个面向企业用户的产品闭环：

```text
Enterprise enrollment
→ Personal / Enterprise ClientRealm
→ A Managed Chat Model
→ B Enterprise Updates Feed / Portal capability
→ C Managed Assistant + Managed Memory Seed + Assistant Starter
→ Enterprise Portal host / status / Enterprise Updates
→ Android enterprise interaction
```

S0.2 不是完整企业平台，也不是 Android 全 profile/Gateway 集成。Enterprise Tool Gateway 属于 S0.3，Android 全 profile 属于 S0.4。

## 2. Entry 与阶段关系

S0.2 Entry：

- S0.1 有效候选证据已固定当前资源协议、A 资源、基础 MCP、Relay、Usage 和 server-side security baseline；
- S0.2 当前 Snapshot `schemaVersion=4` 包含 C 类 Definition；不存在旧 v1 兼容义务；
- Android current reality audit 已确认 Assistant、Quick Message、Memory、MCP 和生成链的唯一 owner/写边界；
- Enterprise Portal 是独立前端产品，Android 只拥有 Native Host、Web Session mint 和受限 bridge。

S0.2 Exit 后，S0.3 先完成 Enterprise Tool Gateway 服务端闭环；S0.4 再完成 Model/TTS/HTTP-ASR/Direct MCP/Gateway 的完整 Android runtime integration、failure/concurrency 和最终设备 Gate。

## 3. S0.2 精确交付内容

### 3.1 A — Runtime Resource

S0.2 最小产品场景要求一个可用的 Managed Chat Model。S0.1 已定义的 Provider/Model/TTS/ASR 继续保留在 Snapshot v4；TTS/ASR 的完整 Android execution Exit 属于 S0.4。

### 3.2 B — Enterprise Capability foundation

S0.2 以 Enterprise Update Feed + Portal 列表/详情交付第一个 B 类用户能力，但不在 Hub 内建立一次性 MCP projection。Control Hub 持有 Enterprise Update authority、Client/Portal Feed 和未来 Gateway 使用的 private typed read semantics。

`get_enterprise_updates` 的模型工具暴露、`discover_tools` / `invoke_tool` 和第三个 daemon 均属于 S0.3。S0.2 不再宣称模型工具调用闭环，也不为过渡期保留 Hub MCP initialize/tools/list/tools/call 旁路。

### 3.3 C — Experience Asset

S0.2 交付：

```text
ManagedAssistantDefinition
Managed Memory Seed embedded in ManagedAssistantDefinition
AssistantStarterDefinition
```

S0.2 不交付 Skill。

### 3.4 Enterprise Update

正式产品名固定为 **Enterprise Update / 企业动态**。它是企业持续发布的动态内容，不等同于要求回执的正式公告，也不局限于宣传新闻。

Enterprise Update 不进入 Managed Snapshot；它使用独立 Feed revision/ETag 和 Client API。更新一条企业动态不得产生新的 `managedGeneration`。

### 3.5 Admin 最小维护闭环

S0.2 Admin 必须支持两条彼此独立的发布路径：

```text
Managed Assistant + Memory Seed + Assistant Starter
→ validate references / preview Snapshot v4
→ Managed Release / managedGeneration

Enterprise Update Draft
→ publish / withdraw
→ Enterprise Update Feed revision/ETag
```

助手、seed 和 Starter 的动态维护通过新 Managed Release 生效；企业动态不等待配置发布，也不得触发 Managed Release。Admin 必须明确展示两者不同的版本与生效状态，不能用一个“保存”动作混淆。

## 4. Snapshot v4 Definition

当前唯一 Snapshot v4 包含 A/B/C 结构与五项必填用户配置准入策略；旧原型不保留兼容或升级路径，版本与缺省值权威为 Control Protocol §10.10.1。以下列出当前快照结构：

```text
ManagedSnapshotV4
  deploymentId
  schemaVersion = 4
  managedGeneration
  releaseId
  snapshotHash
  providers[]
  models[]
  tts[]
  asr[]
  mcp[]
  assistants[]
  starters[]
  policy
  metadata
```

### 4.1 ManagedAssistantDefinition

```text
ManagedAssistantDefinition
  assistantDefinitionId   asd_*
  displayName             non-empty String
  description?            String
  systemPrompt            String
  modelId                 enabled mdl_*
  memorySeed[]            ordered String items; array may be empty, each trimmed item non-empty
  mcpServerIds[]          enabled mcp_* refs
  enabled                 Boolean
```

S0.2 不下发 Android `Assistant` 的完整 Local schema。以下内容不进入 Definition：Local tools、Skill、Workspace、custom headers/body、Regex、Mode Injection、子助手关系、完整 `UIMessage` preset graph、设备主题/背景。

平台 Definition 到客户端运行配置的适配不得扩大企业配置权限。memorySeed 以原助手 ID、generation 和原数组顺序保持身份；实现为显示或索引派生的内部标识不能写入平台 wire，不能转成可编辑运行记忆。配置替换更新 Seed，不覆盖本主体的运行记忆。

S0.2 未下发企业子助手关系；客户端本地示例中的固定子助手扩展不自动变成平台定义，真实来源不得从同名本地助手补入关系。获准用户子助手仍按用户定义与本域授权执行。policy 仅提供 Control Protocol 已定义的默认资源，客户端其他角色选择归本域偏好或既有解析规则；显式失效引用不得按目录第一项或名称静默替换。Starter 选择沿原生 Draft 协议预填并等待用户发送，不增加网页聊天写权限。

### 4.2 AssistantStarterDefinition

```text
AssistantStarterDefinition
  starterId               str_*
  assistantDefinitionId   enabled asd_* ref
  title                    non-empty String
  prompt                   non-empty String
  description?             String
  sortOrder                Integer
  enabled                  Boolean
```

Starter 的 S0.2 语义只限“助手常用入口提示词”。点击后选择对应 Managed Assistant，将 `prompt` 预填到 Draft Conversation 输入框，允许用户修改，并由用户确认发送。S0.2 禁止 auto-send、自动工具调用、表单、附件要求和任务编排。

### 4.3 Managed Memory Seed

`memorySeed[]` 是 Managed Assistant 的只读成熟经验，随 Release/Snapshot 版本化。它与 Android 运行产生的 Enterprise Assistant Memory 永久分离：

```text
Effective Assistant Memory
  = Managed Memory Seed(read-only, generation-bound)
  + Enterprise Local Assistant Memory(mutable user data)
```

Memory tool 只能增删改 Enterprise Local Assistant Memory，不得修改 seed。Definition 更新不覆盖本地运行记忆；seed 更新只随新的 Managed generation 原子切换。

Seed 的作者顺序有语义，不按文本排序或去重；Starter 的客户端展示按 `(sortOrder, starterId)` 确定排序。Snapshot 集合 canonical order/normalization 的唯一权威为 Control Protocol §10.14，不把 UI 展示顺序当作 hash 输入顺序。

## 5. Enterprise Update 与查询合同

### 5.1 EnterpriseUpdate

```text
EnterpriseUpdate
  enterpriseUpdateId eup_*
  title           non-empty String
  content         non-empty text (Markdown)
  contentFormat   MARKDOWN | PLAIN
  category        ANNOUNCEMENT | MAINTENANCE | NOTICE
  severity        INFO | WARNING | CRITICAL
  publishedAt     RFC3339
  status          DRAFT | PUBLISHED | WITHDRAWN
```

Android/Portal Client 只可读取 `PUBLISHED` 内容。

`contentFormat` 声明 content 字段的文本格式；`MARKDOWN` 表示 content 使用 CommonMark 子集（见 §5.3），`PLAIN` 表示纯文本。客户端必须按 `contentFormat` 渲染 content，不得自动推断格式。当 `contentFormat` 缺省时按 `PLAIN` 处理。

`category` 是面向用户的分类标签，只用于 UI 展示和筛选，不影响 Feed revision 或 query projection 语义。`ANNOUNCEMENT` 用于正式公告，`MAINTENANCE` 用于维护通知，`NOTICE` 用于一般通知。

`severity` 表示动态的重要性级别，只用于 UI 展示（如颜色标记），不影响 Feed revision 或 query projection 语义。`INFO` 为常规信息，`WARNING` 为需要注意的变更，`CRITICAL` 为重要紧急信息。

S0.2 不实现标签、评论、阅读回执、定时发布和用户/组定向投递。

### 5.2 Enterprise Update query projection

以下 snake_case 是 S0.3 `get_enterprise_updates` platform tool 的业务投影，不是 Client/Portal HTTP field aliases。S0.2 冻结共同查询语义；公开 Feed 和 private typed HTTP 的字段/ETag 以 Control Protocol §10.16 为准。

输入：

```text
start_date?  YYYY-MM-DD, inclusive
end_date?    YYYY-MM-DD, inclusive
limit?       Integer, default 10, range 1..20
```

固定语义：

- 两个日期都不传：返回最新 `limit` 条；
- 只传 `start_date`：返回该日起至企业当前日期的最新 `limit` 条；
- 只传 `end_date`：返回截至该日期的最新 `limit` 条；
- 两者都传：返回闭区间内最新 `limit` 条；
- `start_date > end_date` 返回 typed invalid-argument error；
- `limit` 小于 1 或大于 20 返回 typed invalid-argument error，不静默 clamp；
- 日期按 Deployment timezone 解释，结果按 `publishedAt` 降序；
- 超过 limit 时返回 `truncated=true`；S0.2 不增加 cursor/search/category。

输出：

```text
enterprise_timezone
items[]
  update_id
  title
  content
  content_format
  category
  severity
  published_at
truncated
```

S0.2 Client/Portal Feed 使用同一 Enterprise Update authority；S0.3 Gateway platform adapter 复用本文日期/limit/timezone/filter semantics，但可以使用面向工具的独立 projection。任何 projection 都不复制内容真源。

Client/Portal 通过公开 Enterprise Update Feed API 读取。Hub 不提供内部 MCP endpoint；S0.3 Gateway 只能调用 private typed read API，不能访问 Admin mutation surface，Android 也不能用 Feed API 冒充一次工具调用。

### 5.3 Markdown 子集

`contentFormat=MARKDOWN` 时，content 使用以下 CommonMark 子集：

````text
 headings (#, ##, ###)
 paragraphs
 bold (**text**) and italic (*text*)
 unordered lists (-, *)
 ordered lists (1., 2.)
 code blocks (```)
 inline code (`code`)
 links ([text](url))
 blockquotes (>)
 horizontal rules (---)
````

不允许 raw HTML、嵌入式图片或任意脚本。客户端在渲染前必须对 content 做 sanitize，防止 XSS。`contentFormat=PLAIN` 时 content 按纯文本原样显示。

## 6. Android 现有内容匹配

| 下发/控制对象 | Android 现有边界 | S0.2 投影规则 |
|---|---|---|
| Managed Provider/Model | Provider/Model picker、generation/render/tool loop | 独立 Managed projection；不写 Local ProviderSetting/apiKey/baseUrl |
| Direct Managed MCP | `McpServerConfig`、`McpManager`、MCP tool loop | 平台 runtime URL/auth；Managed tool cache；不写 Local headers/OAuth |
| Managed Assistant | `Assistant` UI、会话助手和生成配置 | 只读 Managed Assistant read model；不写 Local Assistant aggregate |
| Managed Memory Seed | memory prompt injection boundary | 独立 generation-bound seed store；不写可变 `MemoryEntity` |
| Enterprise Local Assistant Memory | `MemoryEntity` / memory tool | 保持独立可变用户数据，并带 Enterprise Realm/assistant ownership |
| Assistant Starter | `QuickMessage {title, content}` 与 Assistant quick-message UI | 独立 Managed Starter projection；不把 `str_*` 转成本地 UUID |
| Enterprise Update | 无 Settings 对等对象 | 独立 Portal Feed cache/store；不进入 Settings/Snapshot |
| Realm/Binding/Session | 无 Local Settings 对等对象 | 独立 Enterprise state owner；不进入普通 backup/export |

复用现有 UI 和执行边界不等于复用 Local persistence schema。Managed platform IDs 不得 hash/strip 成 Android UUID。

## 7. ClientRealm 与可见性

```text
PERSONAL
  Built-in + Personal Local

ENTERPRISE(deploymentId)
  Managed + policy-allowed User Configuration + Enterprise Preferences
```

五个 `allowLocal*` 分别控制对应类别；全部 false 不能标为全部 A/B/C 的 `Managed Only`。清单外准入、用户主/子助手和引用修正遵循生命周期架构 §4.1。获准用户配置直接复用唯一原定义及凭据，不在企业域另建或复制；旧首期限制不能禁止获准 Provider/MCP/Assistant。不可移除的安全/导航等宿主功能不属于 A/B/C，可选 Built-in 资源仍服从类别规则。

- Personal Realm 不显示 Managed Model/MCP/Assistant/Starter/Enterprise Update 入口；
- Enterprise Realm 中 Managed Assistant、seed、Starter 和 Direct Managed MCP 均只读；
- Realm switch 不改写 Conversation/运行 Memory/Attachment/Workspace 的域与主体归属；用户 Assistant 定义可跨域引用，运行数据分别保存；
- 启用的 Direct Managed MCP 自动启用且不可关闭，仍按助手 `mcpServerIds` 绑定暴露工具；
- 已开始 interaction 捕获 realm/deployment/generation，不随 UI 切换变更。

## 8. 企业接入、Session 与同步触发

Android 通过扫码或粘贴一次性 Enrollment 资料建立 Binding。企业 Session 使用服务端权威七天 rolling idle expiry：Enrollment 成功建立 `now + 7 days` 的 idle expiry；有效 Session 的 Refresh 成功时原子轮换 Refresh Credential 并续至 `now + 7 days`。普通 API、Runtime request、Snapshot/Feed sync 和 Portal load 不直接续期；Personal Realm 或纯后台活动也不续期。

### 8.1 Managed Snapshot correctness triggers

必须检查 Managed State：

1. 首次 Enrollment 成功；
2. 用户进入 Enterprise Realm；
3. 用户主动同步；
4. 每个新企业顶层 interaction 前；
5. 当前处于 Enterprise Realm 且 App 回到前台时按限频检查。

Push/后台周期任务只能提示 freshness，不承担 correctness，也不得通过静默 Refresh 无限延长 Session。

### 8.2 Enterprise Update freshness triggers

检查 Feed revision/ETag：

1. 首次 Enrollment；
2. 打开 Enterprise Portal；
3. 用户刷新企业动态；
4. 当前处于 Enterprise Realm 的限频前台刷新；
5. future Push 到达后重新拉取权威数据。

## 9. Enterprise Portal MVP

正式企业接入与状态入口同时进入 Debug/Release；本地示例可先于真实平台联调交付，遵循 Android Integration Contract §18.1，不能据此宣称 S0.2 Freeze。扫码/粘贴接入与完整本地配置文件导入是不同原生流程，Portal 不持有接入凭据；精确边界见 Control Protocol §8。

S0.2 Portal MVP 由 Android Native Host 和独立 WebView 前端组成：

```text
Native Host
  enterprise identity/session/status
  managed generation / last sync
  Sync configuration
  Refresh enterprise updates
  Open enterprise workbench
  Disconnect

Portal WebView
  enterprise status summary
  Enterprise Updates list/detail
  restricted photo/audio capture and preview
```

Portal 使用短期受限 Web Session，不接触 Android Refresh Credential，不直接写 Android Settings/Room。S0.2 必须交付受信网页请求拍照/录音、原生权限与用户操作、取消及当前请求文档预览的闭环；切域、文档替换或 Session 失效后不向新主体交付结果。具体产品交互由 Portal Product Requirements 定义，消息及媒体协议以 Control Protocol §8 Bridge v3 为权威。任意文件选择和媒体上传业务不属于本期手机能力闭环。

## 10. S0.2 不实现

- Skill delivery；
- Memory/Conversation/Contribution 上传与 User Sync；
- Experience review/promotion/curation；
- Push correctness、通知、任务或 Starter 定向投递；
- Starter auto-send、workflow、form 或 attachment orchestration；
- Enterprise Update 搜索、回执、评论、定时/分组投递（category 和 severity 已在 S0.2 交付）；
- Organization/Group/RBAC/assignment；
- Enterprise Tool Gateway/Catalog/discover/invoke（S0.3）；
- TTS/ASR/Direct Managed MCP/Gateway 的完整 Android runtime completion（S0.4）；
- 完整登录/注销/多企业账号切换体系。

## 11. S0.2 Exit

只有下列全部成立，才允许进入 S0.3：

1. Snapshot v4/OpenAPI/fixtures/generated artifacts 固定 Realm/C experience schema 并可重放；
2. Admin 可发布 Managed Assistant、memory seed、Assistant Starter 和 Enterprise Update；
3. Android 完成 Enrollment、七天 Session、Personal/Enterprise Realm 与独立持久化；
4. Enterprise Realm 可显示并使用 Managed Chat Model、Managed Assistant、seed 和 Starter；
5. Enterprise Update Client/Portal Feed 工作，两日期缺省时按 limit 返回最新内容，并冻结 future Gateway private query semantics；
6. Portal Host 状态/操作按钮、WebView 企业动态列表/详情及拍照/录音权限、取消、结果隔离在真实 Android 设备上验证通过；浏览器桥接替身和本地示例分别报告，不代替真实平台设备证据；
7. Personal Realm 的 picker/tool registry/assistant/starter/portal entry 不出现企业内容；
8. generation/feed update、restart、revoke、Disconnect、offline 不破坏 Local 数据或产生 unsafe replay；
9. Hub 不存在 Enterprise Update MCP projection/过渡旁路，S0.3 Gateway responsibility 清晰；
10. `measix-s0-enterprise-realm-experience-testing-spec.md` required evidence Green；
11. S0.2 Freeze Manifest 固定 architecture/core/android/portal/schema/fixture/scenario identities。
