# S0.1 — Managed Capability Delivery Contract

> 状态：S0.1 Delivery Contract / pre-Android 产品与架构基线  
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`  
> 术语/ID 权威：`../../00-platform/measix-platform-terminology-and-identifier-contract.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> 实施决议：`measix-s0-capability-delivery-implementation-decision.md`  
> 测试权威：`measix-s0-capability-delivery-system-testing-spec.md`  
> 文档职责：定义 S0.1 在 Android integration 之前必须完成的 Managed Capability 产品闭环、首个完整协议 profile、Admin/Runtime/Usage 可用性和 Client Contract Freeze Gate；不维护 executable OpenAPI schema，也不定义 Android 内部实现。

## 1. S0.1 的产品目标

S0.1 要解决一个明确问题：

> 在 Android 开始 Enterprise integration 之前，平台服务端必须已经能够完整回答“企业管理员发布什么、客户端会收到什么、Runtime 如何执行、Usage 如何观察和计价”。

在 Enterprise Delivery 产品分类中，S0.1 只完整交付 A 运行资源（Model/TTS/ASR）、基础 B 企业能力（MCP）与横切 Policy。`Managed Capability` 仍是它们的平台技术总称，不等于狭义 B 类能力。

因此 S0.1 的成功状态不是“后端有 CRUD/API”，而是管理员可以通过真实 Admin Console 完成：

```text
Upstream + Secret
      ↓
Managed Model / TTS / ASR / MCP / Policy
      ↓
Validate
      ↓
Review Client Snapshot
      ↓
Publish
      ↓
Runtime Relay
      ↓
Deterministic / qualified Upstream Adapter
      ↓
Usage Ledger / Pricing / Cost / Diagnostics
```

整个闭环必须在 **不依赖 Android Enterprise implementation** 的情况下可验证。

## 2. Entry Criteria

进入 S0.1 时，S0 Core 至少具备：

- Control Hub 与 Runtime Relay 可独立启动；
- stable ID / Identity / Enrollment / Session foundations；
- Admin session/CSRF foundations；
- Draft/Release/Snapshot compiler foundations；
- RuntimeControlState full apply/status foundations；
- Relay auth/generation/resource/route/credential/stream/cancel foundations；
- request-level Usage spool/ingest foundations；
- SQLite/Ent/Atlas current-schema initialization；
- Admin Quasar shell 与 generated Admin types；
- OpenAPI/fixture/codegen/CI foundations。

若 implementation branch 其中任一项不 Green，先恢复 S0 Core baseline，再开始 S0.1 feature closure。

## 3. 非目标

S0.1 明确不做：

- Android Enterprise Binding/Managed Runtime implementation；
- Embedding resource；
- standalone Image Generation resource/API；
- generic provider custom-header/custom-body DSL；
- User/Group/Organization capability assignment；
- user-managed OAuth credential forwarding for Managed MCP；
- Agent Space、Agent Runtime、Remote Agent、Runtime Hook；
- Managed Assistant/Skill/Starter 等 C 类企业经验交付；
- 企业助手记忆回流、Experience Contribution 提炼、Enterprise Portal 和企业触达；
- SaaS Billing / invoice；
- generic Quota Engine；
- production observability stack replacement（Prometheus/Grafana 等不是 S0.1 产品依赖）。

## 4. 首个完整协议 Profile

### 4.1 决策

当前交付包含 Android 已使用的模型与语音协议，并保持 Relay 的协议无关边界：

```text
Managed Model  → OPENAI_CHAT_COMPLETIONS | OPENAI_RESPONSES | GOOGLE_GENERATE_CONTENT | ANTHROPIC_MESSAGES
Managed TTS    → OpenAI Speech / Gemini TTS / MiMo TTS / 设备系统朗读
Managed ASR    → HTTP 文件转写 / OpenAI 实时识别 / DashScope 实时识别
Managed MCP    → MCP_STREAMABLE_HTTP
```

MCP 是独立标准协议，不属于 OpenAI provider family，但同样属于 S0.1 required Managed Capability。

### 4.2 为什么选 Chat Completions 作为第一条 Model profile

S0.1 的目标是最大化 Adapter compatibility、最小化协议面，而不是追逐最新 Provider-native API。`OPENAI_CHAT_COMPLETIONS` 在第三方 OpenAI-compatible Adapter/Provider 中兼容面更广，Android 当前也有成熟执行路径，因此作为 S0.1 required model profile。

企业模型必须同时可表达 Chat Completions 与 Responses，两者是当前支持范围内的不同线协议，不是旧新版本兼容或失败回退。管理员在 Provider Definition 中明确选择；模型继承该协议，接口路径独立配置。DeepSeek 按其 Chat Completions 接口配置；CLIProxyAPI 可按部署实际支持选择任一协议。不得根据模型名或 Relay 域名猜测协议，不得在调用失败后切换协议。

Responses 的当前客户端执行约定是 HTTP POST + SSE，以完整 `input` 维护对话，`store=false`，不依赖 `previous_response_id` 或供应商会话存储；保留适用的 reasoning items 与 function-call output。客户端负责编码和解析，Relay 仅透明转发，不翻译为 Chat Completions。模型声明 TOOL/REASONING 表示管理员配置的能力，仍需对应供应商协议验收；不是运行时探测结果。共享样例与消费测试必须分别覆盖两种协议，未执行 Android 消费验证不得宣称 S0.4 完成。

当前范围同时包含 Android 已有的 Google Gemini 与 Anthropic Claude 原生模型协议。`GOOGLE_GENERATE_CONTENT` 使用 `contents`/`parts`、对应 tools/functionCall/functionResponse；流式路径为 `/v1beta/models/{实际模型名}:streamGenerateContent`，客户端追加 `alt=sse`，解析 `candidates` 增量。`ANTHROPIC_MESSAGES` 使用 `/v1/messages`、显式 `max_tokens`、messages、独立 system 及 tools/tool_use/tool_result，`stream=true`，解析命名 SSE 事件直至 message_stop。两者均由客户端维护完整对话，不改写为 OpenAI body。供应商认证分别由上游注入 `x-goog-api-key`、`x-api-key`；Anthropic 的 `anthropic-version` 为公开协议头，由客户端按当前协议设置并透明转发。客户端到 Relay 始终使用平台 Bearer，不下发供应商密钥。各协议须独立验证文本及工具调用、错误和取消；支持声明与已验收证据分别记录。

### 4.3 “协议存在”与“产品支持”分离

协议枚举及配置入口不代表供应商或 Android 已 VERIFIED 支持。Admin 暴露当前已实现的 profile，并明确区分配置校验、连接检查与实际调用验收；不能把管理员声明的 TOOL/REASONING 或连通性结果呈现为能力验证成功。未知或未实现协议不提供正常配置入口。

## 5. Managed Resource 模型

S0.1 不把 Android `Settings` / `ProviderSetting` JSON 当 wire。Managed Capability 是独立平台 Definition，再由 Android integration sub-stage 按 ClientRealm 映射到 Effective Runtime。S0.1 的 Release/Snapshot 模型必须保持 typed Definition/Policy 可扩展性，但当前 schema/Exit 不伪装已经支持 C 类 Experience Asset。

### 5.1 Provider Definition

Provider Definition 是客户端侧模型分组/协议身份，不是 Upstream endpoint：

```text
ProviderDefinition
  providerId      prv_*
  displayName
  clientProtocol  OPENAI_CHAT_COMPLETIONS | OPENAI_RESPONSES | GOOGLE_GENERATE_CONTENT | ANTHROPIC_MESSAGES
  enabled
```

Provider Definition 不含：

```text
apiKey
baseUrl
SecretRef
provider-specific admin credential
runtimeRouteId
```

### 5.2 Model Definition

S0.1 Managed Model：

```text
ModelDefinition
  modelId             mdl_*
  providerId          prv_*
  displayName
  upstreamModelKey    opaque adapter/provider selector
  runtimePath         path-only
  inputModalities
  outputModalities
  capabilities
  enabled
```

S0.1 capability vocabulary 收敛：

```text
inputModalities:  TEXT | IMAGE
outputModalities: TEXT
capabilities:     TOOL | REASONING
```

S0.1 的 Model 是 Chat/LLM runtime。Android Local `ModelType.IMAGE/EMBEDDING` 不因此被云端化。

Model Definition 不下发：custom headers、custom body、provider overwrite、Provider API key/base URL。

### 5.3 TTS Definition

```text
TtsDefinition
  ttsId               tts_*
  displayName
  clientProtocol      explicit cloud protocol or SYSTEM_TTS
  protocol fields     see Control Protocol §10.5
  enabled
```

TTS 编辑器先选择 OpenAI、Gemini、MiMo 或系统朗读，再显示该类型必需的字段；不让管理员填写无意义的上游、模型或路径。云端鉴权保留服务端；系统朗读作为企业资源在设备执行，无 Relay 绑定。字段、音频格式及混用拒绝规则由 Control Protocol §10.5 唯一维护，Android 需执行对应消费验证。配置、Snapshot 投影与客户端默认选择必须保留协议差异，不能仅换模型名复用 OpenAI Speech。

### 5.4 ASR Definition

```text
AsrDefinition
  asrId               asr_*
  displayName
  clientProtocol      见 Control Protocol §10.6
  upstreamModelKey
  runtimePath
  language?           optional default language
  enabled
```

ASR 包含 HTTP 文件转写及 Android 已有的 OpenAI/DashScope 实时识别，参数、WebSocket 路径与握手语义统一见 Control Protocol §10.6。管理台按协议显示对应字段，切换时清除不适用参数。

`prompt`、VAD 与采样率只属于对应实时识别协议，不得写入 HTTP 文件转写配置。

### 5.5 MCP Definition

```text
McpDefinition
  mcpServerId         mcp_*
  displayName
  clientProtocol      MCP_STREAMABLE_HTTP
  runtimePath
  authOwnership
  enabled
```

S0.1：

```text
authOwnership = ENTERPRISE_MANAGED | NONE
```

- `ENTERPRISE_MANAGED`：MCP upstream credential 只在服务端 Upstream/Secret 中维护；
- `NONE`：目标 MCP Server 不要求额外 upstream credential；
- `USER_MANAGED` 不进入 S0.1，避免在 Enterprise Runtime Authorization 已占用 Authorization header 时形成未定义的 per-user credential relay 语义。

Android Local MCP OAuth 保持 Local capability，不进入 Managed Snapshot。

`enabled=true` 的 Direct Managed MCP 自动进入所属 Enterprise Realm 可用资源目录，不要求用户复制或逐个开启；S0.2+ 模型工具暴露仍按 Assistant 的 `mcpServerIds[]` 绑定，并非 Gateway 那样的全局内置工具。切回 Personal Realm 后，该 MCP 不出现在 picker、tool registry、resolver 或 execution boundary。

### 5.6 Managed Policy

当前 policy 的五项必填准入、默认资源和用户原配置复用规则统一见 Control Protocol §10.10.1 与生命周期架构 §4。仅当前协议有效，不保留四项旧策略、显式升级或旧 Snapshot 哈希要求。宿主安全和导航功能不得绕过策略。

S0 各 sub-stage 都不做 user/group assignment；Published Managed Snapshot 对 Deployment 中 ACTIVE user/device 一致可见。

## 6. Runtime Binding 与 Client Definition 分离

Resource Definition 回答“客户端看见并怎样调用”，Runtime Binding 回答“服务端把该能力执行到哪里”。两者不能混在一个对象里。

Server-side Draft 必须为 enabled resource 建立唯一有效 binding：

```text
RuntimeBinding
  resourceId
  upstreamId
  allowedMethods
  allowedPathPrefixes
  transportPolicy
  timeoutPolicy
```

发布前须验证客户端定义的 `runtimePath` 位于对应 binding 的 `allowedPathPrefixes` 内。Direct MCP 的 Streamable HTTP binding 包含 `POST`、`GET`、`DELETE`；上游若不提供 GET 事件流可自行返回 405，平台路由不能预先拒绝。

Hub 内部维护 `runtimeRouteId (rte_*)`，但：

- Admin 正常资源编辑不要求用户理解/输入 route ID；
- Android/Test Client Snapshot 不包含 route ID；
- Runtime URL 只使用 `resourceId`。

## 7. Upstream Product Contract

S0.1 Admin 必须能产生 backend validation 真正接受的 Upstream config，而不是只有一个占位表单。

### 7.1 Required fields

```text
name
baseUrl
transportCapabilities
auth
correlationMode
usageCapabilityLevel
timeoutDefaults
status/enabled
```

Auth：

```text
NONE
BEARER          → SecretRef
STATIC_HEADER   → headerName + SecretRef
BASIC           → username + password SecretRef
```

### 7.2 Candidate / Active

```text
Save candidate
  ≠ runtime change

Apply
  → compile controlRevision
  → Relay atomic apply
  → ACK
  → activeConfigRevision changes
```

UI 必须同时显示 candidate revision、active revision、runtime status。

### 7.3 Secret

- Create/Replace value 只写入服务端；
- value 不回显；
- SecretVersion immutable；
- Replace 后必须把新 SecretRef 写入 candidate 并 Apply；
- browser store/localStorage、Snapshot、Usage、log 均不能出现 plaintext。

## 8. Admin Console S0.1 信息架构

一级导航保持：

```text
Overview
Users
Resources
Upstreams
Releases
Usage
System
```

不增加 Organization/Groups/Billing/Fleet 等未来页面。

### 8.1 Resources

必须提供：

```text
Models | TTS | ASR | MCP | Policy
```

每种 Resource 的 editor 必须能完成：

```text
logical identity
protocol-specific minimal fields
Upstream binding
runtime path/transport policy
Enabled
```

管理员不直接编辑 `runtimeRouteId`。

### 8.2 Model editor

至少支持：

```text
Provider
Display Name
Upstream Model Key
Input: Text / Image
Tools
Reasoning
Upstream
Runtime Path
Enabled
```

### 8.3 TTS editor

至少支持：

```text
Display Name
Model
Voice
Upstream
Runtime Path
Enabled
```

### 8.4 ASR editor

至少支持：

```text
Display Name
Model
Optional Language
Upstream
Runtime Path
Enabled
```

UI 明确区分 HTTP 文件转写与实时录音协议，按所选协议展示字段和传输方式。

### 8.5 MCP editor

至少支持：

```text
Display Name
Upstream
Runtime Path
Auth Ownership: Enterprise Managed / None
Enabled
```

### 8.6 Policy editor

可配置 local coexistence + defaults；default reference 必须指向 enabled resource。

## 9. Validate 与 Review & Publish

### 9.1 Validation

Error 至少覆盖：

```text
invalid stable ID/reference
model missing provider
resource missing binding
duplicate binding
missing/invalid upstream
inactive/disabled upstream where contract requires hard failure
unsupported client protocol
invalid runtime path/method/transport
invalid/missing SecretRef
invalid default resource
snapshot secret/internal field leak
```

Warning 至少覆盖：

```text
upstream currently unhealthy/unverified
semantic usage capability insufficient
pricing missing
optional capability not qualification-verified
```

### 9.2 Review

Publish 前展示：

```text
Added / Changed / Removed resources
Policy changes
Upstream/runtime impact
Warnings requiring acknowledgement
```

### 9.3 Client Snapshot Preview

必须由 **与正式 Snapshot compiler 同一 canonical projection** 生成：

```text
Client receives:
  providers
  models
  tts
  asr
  mcp
  policy

Client never receives:
  upstreamId
  Upstream base/internal URL
  Secret / resolved credential
  runtimeRouteId
  Pricing Rule
```

前端禁止自行拼一个“看起来像 Snapshot”的 preview DTO。

## 10. Snapshot Freeze Contract

当前版本由 Control Protocol §10.10.1 定义。MEASIX 尚未发布，只维护当前完整结构和候选证据，不保留旧原型迁移、兼容或冻结哈希义务。

Freeze 输入至少记录：

```text
platformCoreCommit
clientControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
```

协议修订先更新架构权威，再同步可执行结构、样例、消费者和受影响 gate。

## 11. Runtime Execution Contract

Test Client/未来 Android 请求：

```text
/runtime/v1/resources/{resourceId}{runtimePath}
Authorization: Bearer <enterprise session access token>
X-Measix-Managed-Generation: <captured generation>
X-Measix-Interaction-Id: int_* or test equivalent defined by fixture/harness
```

Relay admission 顺序必须保证：

```text
Auth
→ Principal status
→ Generation Barrier
→ Resource existence/authorization
→ Route resolve
→ Header/Credential policy
→ Upstream body forward
```

任何 admission failure 在 body forward 前完成。

S0.1 Test Client 可以使用 test-only interaction correlation，但不能创造 production-only wire field；正式 client headers 仍以 Control Protocol 为准。

## 12. Transport Acceptance

Required profile 对应 transport：

| Capability | Required transport |
|---|---|
| Model | HTTP request/response + streaming/SSE |
| TTS | HTTP request + binary response |
| ASR | multipart upload + JSON response |
| MCP | MCP Streamable HTTP |

必须覆盖 cancellation、timeout、4xx/5xx、content type、stream flush、binary integrity、multipart integrity。

当前实时 ASR 扩展必须验证真实 WebSocket 升级、双向帧、鉴权与 generation 拒绝、取消/关闭、超时、传输限制及连接计量；HTTP 上传测试不能代替这些证据。

## 13. Usage / Cost Product Contract

### 13.1 Request facts

Relay 对成功解析可信 Principal 的 runtime request 产生 request fact；未认证拒绝不伪造用户 Usage。字段精确 required/optional 规则以 Control Protocol §18 为准，以下是已解析 Upstream route 的概念示例：

```text
requestId
interactionId?
userId
deviceId?
resourceId
upstreamId
managedGeneration
controlRevision
startedAt/completedAt
forwarded
http/upstream status
requestBytes/responseBytes
duration
errorClass?
```

### 13.2 Resource classification

Admin 必须能明确把 request 分类为：

```text
MODEL | TTS | ASR | MCP
```

实现可以依据稳定 resource identity/catalog 关联，不要求 Relay 做 Provider body parsing。

### 13.3 Semantic meter

标准 meter：

```text
MODEL: INPUT_TOKENS / OUTPUT_TOKENS / CACHED_TOKENS / REQUESTS
TTS:   CHARACTERS / AUDIO_SECONDS / REQUESTS
ASR:   AUDIO_SECONDS / REQUESTS
MCP:   REQUESTS
```

Provider/Adapter 还可报告 namespaced extension，但不能覆盖标准语义。

### 13.4 Completeness

```text
EXACT
PARTIAL
UNKNOWN
```

请求只有 request facts、不具备 semantic meter 时依然是合法、可观察的 Usage；不能伪造 token/audio/cost。

### 13.5 Pricing

S0.1 必须能通过 Admin 维护 `PricingRule`，至少按：

```text
resource/upstream scope
meter
unitSize
unitPrice
currency
effectiveFrom
```

计算结果：

```text
KNOWN | PARTIAL | UNKNOWN
```

不生成 invoice，不做 subscription/billing account。

## 14. Usage Admin UX

### Summary

至少展示：

```text
Requests / Errors / Blocked
Model token meters
TTS characters/audio
ASR audio duration
MCP requests
Known/Partial/Unknown usage count
Cost status/amount where valid
```

### Filter

至少支持产品语义上的：

```text
Time
User
Resource / Resource Kind
Upstream
Status
Usage completeness
```

具体 query parameter name 属于 Control Protocol/OpenAPI 下游定义。

### Request Detail

至少展示：

```text
requestId
interactionId?
User / Device
managedGeneration
Resource
Upstream
forwarded
status / duration
semantic meters
usage source/completeness
cost source/status
```

不显示 prompt/body/Secret。

## 15. Overview / System S0.1 可观测性

管理员必须能回答“现在 Managed Runtime 是否真的可用”：

```text
active managedGeneration
runtime READY/ACTIVATING/DEGRADED
Hub desired controlRevision
Relay applied controlRevision + bundle hash
current/last Activation
Upstream active/degraded status
request usage ingest lag/spool state
semantic usage orphan/unknown status
recent activation/runtime failures
```

S0.1 不要求建立独立 metrics platform；Admin read model + logs + system test evidence 足够。

## 16. Deterministic Test Client

为了证明“服务端已经准备好让 Android 消费”，`platform-core` 必须有 test-only Snapshot/Runtime Test Client：

职责：

```text
discovery/control calls needed by harness
fetch Managed State/Snapshot
validate snapshot schema/hash/reference
select required resource
construct required reference-profile request
call Runtime Relay
assert response/stream/binary/multipart/MCP behavior
```

Test Client：

- 不成为 production component；
- 不复制 Android Local Settings；
- 不绕过 public client/runtime topology；
- 只验证 S0.1 frozen client-facing contract。

## 17. Deterministic Test Adapter

Test Adapter 必须提供：

```text
OpenAI-compatible chat response
OpenAI-compatible chat streaming
OpenAI-compatible speech binary
OpenAI-compatible transcription multipart
MCP Streamable HTTP
configurable 4xx/5xx/timeout
cancellation observation
request/correlation capture
```

它不进入 production Adapter list，不承担真实 Provider correctness。

## 18. Real Adapter qualification

S0.1 RC 还必须选择至少一个真实 OpenAI-compatible Adapter/endpoint 验证 required Model profile；若同一 Adapter 提供 Speech/Transcription，可一起验证；否则允许为 TTS/ASR 使用另一已 qualification 的 OpenAI-compatible endpoint。

Architecture 不绑定 CLIProxyAPI/LiteLLM/New API 任一品牌。选择哪个真实 Adapter 是 implementation/qualification input，不进入 Relay branching。

MCP 使用一个真实或受控标准 Streamable HTTP server 验证协议。

## 19. S0.1 Exit Gate

只有以下全部满足，才允许开始 S0.2：

1. `S0 Core` 相关 CI Green，无未解释 critical failure；
2. Admin 可在空环境创建有效 Upstream + Secret + Apply；
3. Admin Resources 五 tab（Models/TTS/ASR/MCP/Policy）完整可用；
4. required profile 以 typed/enforced semantics 表达，不依赖自由字符串和隐式客户端默认值；
5. Resource→Upstream binding 可通过 UI 完成，不需要手写 JSON；
6. Pricing 可维护；
7. Validate/Review/Publish/Activation refresh recovery 可用；
8. Client Snapshot Preview 与实际 Snapshot canonical projection 同源；
9. Test Client 能执行 Model streaming、TTS binary、ASR multipart、MCP Streamable HTTP；
10. old generation/invalid auth/resource deny 均在 Upstream forward 前失败；
11. Relay cancellation/timeout/stream/binary/multipart 通过 system scenarios；
12. Usage 可按 Model/TTS/ASR/MCP、User、Resource、Upstream、Status、Completeness 观察；
13. semantic completeness 正确表达 EXACT/PARTIAL/UNKNOWN，cost status 正确表达 KNOWN/PARTIAL/UNKNOWN；
14. Secret/Internal route/Upstream URL 不进入 Snapshot；
15. OpenAPI/fixture/codegen/hash/ETag tests 无 drift；
16. browser Admin E2E + real Hub/Relay + deterministic Adapter/Test Client system gate Green；
17. 至少一个真实 OpenAI-compatible Adapter 的 required profile qualification 通过；
18. 生成并保存 S0.1 freeze manifest：commit/OpenAPI hash/fixture hash/schema version/scenario result。

未满足任一项，不得用“Android 实现时再补”关闭 S0.1。

## 20. S0.1 完成后的允许变化

§4 已明确的四种模型、四种 TTS 与三种 ASR 属于当前交付范围，不是等待 S0.1 Exit 后再做的功能。后续仍可增加当前范围之外的 profile 或 provider-specific semantic Usage Connector。

但必须满足：

- 作为后续明确约定的 profile extension，不预设旧原型兼容承诺；
- 资源协议保持当前明确语义，旧原型不构成兼容义务；
- 有明确 client capability compatibility；
- 有 Adapter qualification；
- 不阻塞 S0.2、S0.3、S0.4 或 S1。

新增 profile 先更新架构与 executable contract，再同步共享样例及消费者。当前产品从未发布，不增加旧原型兼容或自动回退路径。
