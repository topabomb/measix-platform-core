# S0.4 — Android Managed Runtime Integration Contract

> 状态：S0.4 Delivery Contract + Android Component Architecture Authority
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`  
> 企业经验上位边界：`../../00-platform/measix-enterprise-experience-lifecycle-architecture.md`
> 前置 Gate：S0.3 Enterprise Tool Gateway Freeze
> Wire 权威：`measix-s0-control-protocol.md`  
> Android 测试：`measix-s0-android-client-testing-spec.md`  
> 最终系统测试：`measix-s0-system-testing-spec.md`  
> 实现仓库：`topabomb/rikkahub_mcp`  
> 文档职责：统一定义 S0.4 Android 的 entry/exit、Enterprise Binding、Managed State/Snapshot、Effective Runtime、interaction correctness、Direct MCP/Gateway mapping 和用户边界；不固定 Kotlin package/class/file、DataStore/Keystore wrapper、DI module 或具体 HTTP helper。

## 1. S0.4 目标

S0.4 的入口要求 S0.1 Direct Managed runtime、S0.2 Snapshot v4/ClientRealm/Experience/Portal 和 S0.3 Snapshot v5/Gateway baseline 分别通过冻结门槛；此处描述阶段依赖，不宣称当前已冻结。历史版本与当前修订的对应见 Control Protocol §10.10.1。

S0.4 的目标：

```text
ClientRealm
  ├─ PERSONAL   → Built-in + Personal Local → existing Local Runtime
  └─ ENTERPRISE → S0.3 frozen Managed Snapshot v5
                     + policy-allowed User Configuration + Enterprise Preferences
                     → Managed interaction correctness boundary
                     → existing Model / TTS / ASR / Direct MCP execution boundaries
                     → governed Gateway discover/invoke boundary
                     → Runtime Relay
```

用户在 Personal Realm 只看到 Built-in/用户配置；进入 Enterprise Realm 后，在同一套现有资源 UI 中看到 Managed 与 Policy 允许的用户原配置共存。Managed 资源明确标记只读/受管；切回 Personal Realm 后企业内容不可见、不注册、不可执行。

Android 不是 thin client，也不建立第二套 Enterprise Chat/Conversation/Provider stack。

## 2. Entry Gate — valid S0.3 Freeze required

开始 S0.4 Android 全 profile 集成前必须固定（S0.2 的最小 Android Realm/Experience 实现仍按其自身 Entry 开始，不倒置阶段依赖）：

```text
architectureCommit
platformCoreCommit
clientControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
gatewayControlOpenApiHash
gatewaySurfaceHash / evaluationCatalogHash
S0.3 scenario/evidence manifest
real Adapter qualification evidence relevant to claimed profile
```

禁止把 floating `platform-core latest` 作为 integration contract。

若 S0.3 Freeze 未 Green，S0.4 不开始。若 frozen v5 无法表达 required Android execution semantic，必须按 compatibility/versioning 规则回到 architecture/Control Protocol 处理，不能在 Android 本地发明 field/fallback。

## 3. Required Managed Profile

S0.4 只要求消费 S0.3 已 Frozen、并继承 S0.1/S0.2 baseline 的 required profile：

```text
Model  → OPENAI_CHAT_COMPLETIONS
TTS    → OPENAI_AUDIO_SPEECH
ASR    → OPENAI_AUDIO_TRANSCRIPTIONS
Direct MCP → MCP_STREAMABLE_HTTP
Gateway    → MCP_STREAMABLE_HTTP with fixed discover_tools/invoke_tool surface
```

不要求：native Anthropic/Google/Gemini、OpenAI Responses、Realtime/WebSocket managed runtime、Embedding、standalone Image Generation 等 future profile。

未来 profile 只有在 server contract + Adapter qualification + Android compatibility 都明确后增量加入。

## 4. Local-first 与 ownership

Android 保持现有本地主链完整：

```text
ChatService / Conversation lifecycle
→ existing Generation / Tool Loop
→ existing provider/runtime implementations
```

Enterprise 只在稳定边界增量接入：

```text
new top-level Managed action
→ Enterprise interaction gate
→ authoritative managed-state preflight
→ snapshot sync if required
→ capture immutable interaction runtime context
→ existing execution boundary
→ managed endpoint/auth adaptation
→ Runtime Relay
```

Control Hub 不成为 Chat Runtime；Android 不上传 Conversation/Message 作为 S0 correctness dependency。

## 5. State ownership：用户配置、偏好与企业下发配置

持久/计算模型：

```text
EffectiveRuntime(PERSONAL)
  = Built-in Defaults + Personal Local Settings

EffectiveRuntime(ENTERPRISE, deploymentId)
  = Applied Managed Snapshot(deploymentId)
  + policy-allowed User Configuration + Enterprise Preferences
```

Enterprise Realm 的用户及可选 Built-in 配置按生命周期架构 §4.1 分类别准入，五项之外的明确允许项继续可用；Built-in 资源不绕过对应类别开关。不可移除的安全、导航和宿主能力不属于可选配置。

规则：

- Managed Snapshot 不写入 Local Settings；
- Local records 不因为 Managed shadow 而删除/改写；
- 用户资源只保存一份，企业域获准后直接引用原定义和用户凭据，不注入企业 Session，不上传凭据；运行数据另外按域与主体隔离；
- 引用携带来源及稳定身份，Managed 同 ID 不覆盖用户定义；
- Managed remove/Disconnect 后用户配置和 Personal 数据不受破坏；企业运行数据的保留/导出策略单独明确；
- Managed resource mutation 必须在 write/commit boundary 被拒绝，不能只靠 UI disabled；
- credential/session state 与 Local backup/export 分离；
- Managed state corruption/sync failure 不得污染 Local Runtime。

`origin` 与 `realm` 分开表达：

```text
origin = BUILT_IN | LOCAL | MANAGED
realm  = PERSONAL | ENTERPRISE(deploymentId)
```

Conversation、运行 Memory、Attachment 和 Workspace 必须保留可确定的域与主体归属。对象一旦创建，切换 ClientRealm 不得隐式改变归属。用户 Assistant 定义只有一份，跨域运行时分别读取和写入本域主体数据。

### 5.1 派生生效配置与执行边界

遵循生命周期架构 §4 的三类配置归属及五项准入策略。生效配置是唯一派生读模型，不另持久化第四份配置真源；包含域、企业、适用版本、资源/助手/工具、实际选择与参数、来源、可选择/可修改/强制启用/不可用原因及显示操作偏好。修改写回对应 owner，再整体重新计算。

UI、命令和执行使用同一规则，写入与执行入口重新校验。用户助手允许使用不等于引用资源全部可用；不合规引用必须显示原因并阻止执行，不能回退到首项或替换为其他来源。运行开始捕获完整执行配置和域/主体，后续选择变化、切域或配置更新不能改写已捕获运行。

### 5.2 当前配置初始化与数据归属

MEASIX 尚未发布，Android 只实现当前完整配置与数据结构。非当前开发配置或数据库直接清理后重新初始化，不提供字段转换、版本回填、双读、双写或第二配置真源。物理存储布局由 Android 仓库决定，但不能改变三类配置 owner 和派生生效配置边界。

新增运行数据必须有域和主体归属后才能写入。个人备份/恢复只处理个人范围，不能用整库或整目录替换覆盖企业数据、企业凭据与下发配置。验证覆盖 clean initialization、Realm/主体必填、个人恢复保全企业数据，以及不存在旧格式读取入口。

## 6. 三个独立状态维度

Identity binding、Managed Runtime readiness 与当前 ClientRealm 不合并成一个 enum。

### BindingState

```text
UNBOUND
  → ENROLLING
  → BOUND

BOUND → REVOKED
BOUND → explicit Disconnect → UNBOUND
REVOKED → clear/re-enroll → UNBOUND/ENROLLING
```

### ManagedRuntimeState

```text
UNKNOWN
→ READY
→ SYNC_REQUIRED
→ SYNCING
→ READY | SYNC_FAILED

READY/SYNC_* → CONTROL_UNAVAILABLE
CONTROL_UNAVAILABLE → successful authoritative preflight → READY/SYNC_REQUIRED
```

`BOUND` 不自动意味着 Managed Runtime READY。

### Active ClientRealm

```text
PERSONAL
  ↔ ENTERPRISE(deploymentId)
```

- Personal Realm 总是本地可进入；
- 进入 Enterprise Realm 需要 `BOUND`；发起新 Managed runtime 还需要 Managed Runtime preflight 通过；
- `BOUND` 时用户仍可处于 Personal Realm；
- revoke/Disconnect 使 active realm 回到 PERSONAL，不删除 Personal Local 数据；
- 切换 realm 不中途改变已开始 interaction 的 captured realm/generation/effective runtime。

## 7. Installation / Enrollment / Binding

Android 自己生成的长期 client-side correlation identity 只有 Terminology Contract 指定的 installation/interaction 类型；server User/Device/Session/Resource identity 必须由 Hub返回并原样持久/传播。

Enrollment 流程：

```text
normalize platform origin
→ discovery
→ exchange one-time enrollment code
→ receive/persist Enterprise Binding + refresh credential
→ bootstrap
→ read managed state
→ sync Snapshot when required
→ BOUND / READY
```

Client request 不提交任意 userId 来选择企业身份；Enrollment code 已绑定 authoritative user context。

Platform URL 必须是受支持的 secure origin，不允许 Snapshot/runtime data 引入 arbitrary provider host。

## 8. Credential / Session

要求：

- Refresh credential 使用 Android platform secure storage / Keystore-backed protection；
- Access Token 只驻进程内存；
- concurrent access refresh single-flight；
- Control request 401 只在协议允许且 request 无外部副作用时 refresh once + retry once；
- 已开始 Runtime POST/stream/tool/TTS/ASR/MCP request 发生 auth/network uncertainty 时不自动 replay；
- Enterprise credential 不写入 Local Provider `apiKey`/custom header config；
- Disconnect 清 Enterprise Binding/Credential/Managed state，不删除 Local settings/history。

产品级 Enterprise Session 采用服务端权威七天滚动闲置时限：Enrollment 初始化 expiry，只有成功的 authenticated Refresh 原子轮换 Refresh Credential 并续期；普通 API/Runtime/同步/Portal load 不单独续期，Android 不用后台 heartbeat 保活。闲置超过七天后需重新验证/接入。这是 Session 语义，不要求 Access Token 本身持续七天；Device revoke 和 explicit Disconnect 可以提前终止 Session。

在 Android implementation 前，Control Protocol 必须先固定续期触发、服务端权威时钟、客户端可观察 expiry 和 Refresh Credential rotation/reuse 语义；客户端不得自行延长本地时间或把一个固定 `refreshExpiresAt` 猜成滚动会话。

具体 Keystore API、DataStore schema、HTTP class 由 `rikkahub_mcp` implementation 决定。

## 9. Applied Managed State

Android durable Managed state 必须以一个 crash-safe whole-state unit 表达：

```text
snapshot payload
appliedManagedGeneration
snapshotHash
schemaVersion
releaseId
validated managed-state metadata needed for recovery
```

必须候选下载/解码/验证全部成功后再原子替换当前 Applied state。

禁止：

```text
先写 generation
→ 再写 payload
```

任何失败保持 Last Known Good，不产生 metadata/payload half commit。

`managedGeneration=0` 表示 server 尚无 active Release；Client 可以 BOUND，但不得伪造 generation-0 Snapshot。

## 10. Managed State Guard

BOUND 状态下，每个**新的顶层 Managed Runtime interaction** 开始前必须获得 authoritative managed-state correctness：

```text
GET /api/client/v1/managed/state
```

决策语义：

```text
READY + same generation → allow
READY + mismatch        → sync required
ACTIVATING              → block/update message
DEGRADED                 → block new Managed runtime
401                      → safe control refresh once + retry control call
revoked                  → REVOKED
network/5xx              → CONTROL_UNAVAILABLE / fail closed for new Managed runtime
```

S0.4 不使用 TTL cache 跳过 correctness preflight。

并发 interactions 可以共享 single-flight preflight/snapshot sync network work，但每个 interaction 成功后必须拥有独立 immutable context。

Local-only actions/history browsing 不因为 enterprise control outage 被全局锁死。

## 11. Snapshot Sync / validation

流程：

```text
resolve target generation
→ download Snapshot / observe ETag/hash contract
→ decode frozen supported schema
→ verify deployment identity
→ validate stable IDs/references
→ validate required clientProtocol/profile fields
→ build candidate Effective Runtime
→ atomic Applied Managed State commit
→ publish new Effective Runtime View
→ READY
```

Validator 至少阻止：

- unsupported schema/protocol 被 silent fallback；
- broken Provider/default/resource references；
- invalid runtimePath / arbitrary host；
- TTS missing voice；
- unsupported Managed realtime ASR config；
- unsupported MCP auth ownership；
- Secret/Upstream/internal route server-only material 泄漏；
- hash/deployment mismatch。

Invalid candidate 保持 LKG；但 LKG 不能绕过 Relay active-generation barrier。

## 12. Effective Runtime / origin / lock

Effective Runtime 必须能区分：

```text
origin = BUILT_IN | LOCAL | MANAGED
realm  = PERSONAL | ENTERPRISE(deploymentId)
```

并维护足够的 origin/lock information，使现有 picker/settings/execution boundary 能：

- Personal Realm 只展示 Built-in + Personal Local；
- Enterprise Realm 在同一列表中展示 Managed + policy-allowed User Configuration；
- Managed 清楚标记企业来源；
- Managed edit/delete command 被拒绝；
- 用户原定义修改写回用户配置 owner；企业策略只限制本域使用，不锁住个人配置修改；受管定义不可编辑；
- Managed shadow/remove/disconnect 不破坏 Local record；
- default/availability 依据当前 effective policy 决定。

不要为 Enterprise 复制第二套 settings catalog 或 picker；ClientRealm 决定同一套 UI 当前投影哪个作用域。

## 13. Interaction Runtime Context

每个 top-level Managed action 成功通过 Guard 后 capture：

```text
interaction identity
client realm / deployment identity
managed generation
effective runtime snapshot/reference needed for deterministic execution
start time / correlation metadata
```

一个 interaction 内的 Chat/Tool loop 使用同一 captured generation/effective runtime，不因后台 sync 中途切换。

独立 TTS/ASR/MCP top-level action 创建自己的 context；若某操作明确是父 interaction 的子操作，则调用层显式传递 parent context。

禁止依赖一个全局 mutable “current interaction” 猜测并发归属。

## 14. Managed Runtime endpoint / auth

Managed Resource 只通过平台 Runtime surface 执行：

```text
{platform origin}/runtime/v1/resources/{resourceId}{runtimePath}
```

Android 注入 Control Protocol 定义的 enterprise runtime auth、managed generation 和 interaction correlation。

禁止：

- 使用 Snapshot 中的 arbitrary base URL；
- 把 Enterprise access token 写入 Local Provider apiKey；
- Client 知道 runtimeRouteId/upstreamId/server credential；
- lower-level helper 暴露一条绕过 Managed State Guard 的 top-level execution path。

## 15. Required profile mapping

### 15.1 Model

`OPENAI_CHAT_COMPLETIONS` 应复用现有 compatible model execution path；Managed projection 提供 frozen Model identity/selector/capabilities，而网络 target/auth 由 Managed Runtime boundary 替换。

TEXT/IMAGE input、TEXT output、TOOL/REASONING 只按 Snapshot 声明的 capability 生效；不得从 Local provider config 偷默认值补齐 server contract 缺失。

### 15.2 TTS

`OPENAI_AUDIO_SPEECH`：请求必须使用 Snapshot 的 upstream model selector + **voice**；binary MP3 response 进入现有 playback/save pipeline，不让 Local voice fallback 覆盖 Managed Definition。

### 15.3 ASR

`OPENAI_AUDIO_TRANSCRIPTIONS`：使用 HTTP multipart file + model + optional language；不要求/伪造 realtime WebSocket/VAD/sample-rate config。Local realtime ASR 继续作为 Local capability。

### 15.4 Direct Managed MCP

`MCP_STREAMABLE_HTTP`：复用现有 compatible Streamable HTTP boundary；`ENTERPRISE_MANAGED` 只发送平台 Runtime auth，实际 upstream credential 仍由 Relay server-side 注入；`NONE` 不制造 credential。Local OAuth/header MCP 继续 Local。Direct Managed MCP 继续按 Assistant `mcpServerIds[]` 获得真实工具定义。

### 15.5 Enterprise Tool Gateway

Snapshot v5 的 `ToolGatewayDefinition` 是受管 logical resource，不是用户可编辑的 MCP Server。Android 必须通过标准 MCP initialize/`tools/list` 读取且只接受 `discover_tools`、`invoke_tool`，按 canonical bytes 校验 Snapshot `surfaceHash`；不得从 Snapshot 本地拼接工具定义或在 mismatch 时 fallback 到旧 cache/Direct MCP。

Enterprise Realm effective tool set：

```text
Android required host/safety tools
+ discover_tools / invoke_tool as one atomic built-in pair when Gateway published, surface verified, and effective client policy enabled
+ Assistant-bound Direct Managed MCP tools
+ explicitly admitted user tools (user MCP follows allowLocalMcp)
```

Gateway pair 的 effective enablement 固定为：

```text
REQUIRED
  → enabled；用户不能关闭

USER_CONTROLLABLE_DEFAULT_ON
  → Enterprise-local preference if present, otherwise enabled
```

本地 preference 按 `(deploymentId, toolGatewayId)` 保存，与 Applied Managed State 和普通 MCP Settings 分离，不上传服务端、不改变 `managedGeneration`、不跨 Deployment 复用。服务端切到 REQUIRED 时已有 false preference 保留但不生效，切回可选时恢复其效果；客户端不得用该偏好阻止 Snapshot apply 或 Gateway surfaceHash 校验。

缺失/未知 `clientEnablementPolicy` 使候选 Snapshot 验证失败。本地启停只影响下一 interaction capture，不修改在途 context/tools；立即停止使用显式 cancellation。服务端 policy 改变随新 generation 生效，旧 interaction 下一请求仍受 428 barrier 约束。

Personal Realm 不注册/解析/执行 Gateway 或 Direct Managed MCP。`discover_tools` 返回的真实工具合同只属于 captured interaction；`toolRef` 不跨 interaction/generation 持久复用。`invoke_tool` 的 resolved business action/status 必须投影到现有工具卡/诊断，不让用户永远只看到通用 `invoke_tool`，也不显示 private integration endpoint/credential。

## 16. 428 / revoke / failure behavior

只有 Control Protocol 明确表示 `managed_snapshot_required` 且 `forwarded=false` 的 generation mismatch 才驱动 Snapshot update path。

规则：

- 当前 interaction/request 终止，不自动 replay；
- sync 完成后下一次新的 user/top-level action 使用新的 interaction identity/generation；
- 401/403/network error 如果 request 可能已经产生 upstream side effect，不能自动重放；
- revoke 进入 REVOKED，不能本地 toggle 回 ACTIVE；
- control unavailable 时 fail closed for new Managed runtime，但 Local/history 不被无关阻断。

## 17. UI boundary

Enterprise Settings 至少表达：Binding state、Deployment/User/Device identity summary、managed generation/readiness、sync/retry、Disconnect。

Android 顶层体验至少表达：

- Personal / `<Enterprise display name>` realm 切换；
- Personal Realm 不出现 Managed 资源、工具、助手或场景入口；
- Enterprise Realm 显示企业来源、受管/强制启用状态及用户原配置准入和不可用原因；
- 扫码/粘贴称为“接入企业”，不要求首期建设完整账号登录/注销体系。

现有 Resource UI：

- 获准用户原配置 + Managed 同一列表/picker；
- Managed 有清楚的 “Managed by <Deployment>”/lock/origin；
- Managed mutation disabled + command boundary deny；
- 不出现第二套 Enterprise-only picker/catalog；
- unavailable/update/revoked/control-outage 有明确可恢复状态。

Gateway 不出现在 MCP Server 设置、URL/Header/OAuth editor 或普通连接管理。Enterprise Settings 可以显示 Gateway readiness/generation/surface mismatch；当 policy 为 `USER_CONTROLLABLE_DEFAULT_ON` 时可显示一个控制整对 Gateway 工具的企业能力开关，为 `REQUIRED` 时显示受管锁定状态并拒绝写入。工具调用必须显示真实业务动作。Portal Native Host、状态/操作、动态及相机/麦克风交付由 S0.2 产品合同定义；S0.4 保留这些实际验收后的回归证据，不把尚未完成的手机能力称为已验收。

## 18. Persistence / backup / privacy

- Enterprise credential/session 不进入普通 Local Settings export/backup；
- Managed Snapshot 可以按 client-safe contract 持久化，但与 credential store 分离；
- Gateway Enterprise-local enablement preference 与 Managed Snapshot、普通 MCP Settings/backup 分离，并按 Deployment/Gateway scope 保存；
- process restart 恢复的 Applied state 必须保持 whole-state consistency；
- corrupt/unsupported local Managed state 不覆盖 LKG/Local state；
- log/crash report 不含 token/Secret/full sensitive prompt/body。

### 18.1 正式企业入口与本地示例服务

Android 在 Debug 和 Release 均提供正式空间入口和原生企业接入/状态页，可接入示例企业、扫码或粘贴资料、同步、切回个人和退出；不依赖开发模式或隐藏开关。本地企业持续显示“本地模拟”及来源，不因示例功能存在而自动接入。接入资料和本地/平台来源边界遵循 Control Protocol §8。

本地示例只替换企业外部服务 I/O，复用正式 Realm、配置解析、写入校验、页面、执行和持久化流程，不创建第二套模拟配置解析或聊天运行链。内置示例绑定使用确定性服务，不需要真实凭据；策略允许且用户主动选择的已有资源沿用户执行链使用其原凭据，不能暗中改成模拟回复。显式导入的本地企业真实绑定标明连接来源，失败不回退为示例，也不把模拟凭据发送到真实平台。不覆盖真实企业绑定；自动化模拟验收使用隔离测试资源/传输替身，证明无意外真实请求，不能把“防止意外调用”解释为修改用户资源的正常语义。

必须可验证：个人/企业域切换；企业资源、企业助手、强制启用 Direct MCP；五个开关允许、禁止、收紧及恢复和清单外准入；已有用户资源/助手的选择与对应执行来源；配置更新、网络失败、凭据失效、退出及进程重启；Portal 原生宿主和手机能力调用。来源命名空间、企业/用户、配置版本和执行归属在重启后保持，不自动把本地身份升级为真实身份。两种构建均需入口证据，真实平台与本地示例结果分别报告；本地示例不替代真实 Android/Hub/Portal 系统验收。

## 19. 明确非目标

S0.4 不实现：

- 第二套 Enterprise Chat/Conversation Runtime；
- server-side Conversation persistence；
- native Anthropic/Google/Gemini required support；
- OpenAI Responses required support；
- Managed Realtime/WebSocket ASR；
- Embedding/Image Generation standalone Managed API；
- MDM uninstall/disconnect enforcement；
- FCM correctness dependency；
- Agent Space/Remote Agent/Runtime Hook；
- per-user/group capability assignment；
- Managed Skill 正式交付；
- S0.2 Portal MVP 之外的 WebView/native bridge 业务能力；
- Conversation/Memory/Experience Contribution User Sync；
- 企业通知、Starter 定向投递和任务。
- Gateway write/destructive tools、用户审批、per-user OAuth/RBAC 或 Direct/Gateway fallback。

## 20. S0.4 Exit

Android S0.4 可以进入 Final S0 System/RC Gate，仅当 `measix-s0-android-client-testing-spec.md` 证明：

1. valid frozen S0.3 Snapshot v5/Gateway contract 被 pin/reproducibly consumed，S0.1–S0.3 baseline 保持 Green；
2. Enrollment/Binding/Credential/Managed State crash-safe；
3. Personal/Enterprise 运行数据隔离、获准用户原配置与 Managed 共存、五项准入及引用校验正确，当前结构初始化与个人恢复不破坏企业数据；
4. every new Managed top-level interaction 通过 authoritative correctness guard；
5. required Model/TTS/HTTP-ASR/Direct MCP/Gateway 全部映射现有 runtime boundary，surfaceHash/真实工具 UI 正确；
6. 428/revoke/control outage/cancel/auth uncertainty 不产生 unsafe replay；
7. managed endpoint/auth 不能被 Local arbitrary host/credential 绕过；
8. real emulator/device T4 lane 可以从 Enrollment 开始完成 required runtime path；
9. Local existing runtime regression 保持 Green。
