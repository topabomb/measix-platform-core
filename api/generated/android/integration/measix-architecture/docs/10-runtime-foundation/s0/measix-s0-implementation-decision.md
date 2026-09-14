# S0 实施顺序与技术选型决议

> 状态：S0 Implementation Decision / 跨组件实施决议基线  
> 版本：2026-08-30
> 上位文档：`measix-s0-foundation-contract-spec.md`  
> 文档导航：`../../measix-documentation-guide.md`  
> 关联文档：`measix-s0-capability-delivery-implementation-decision.md`、`measix-s0-enterprise-realm-experience-contract-spec.md`、`measix-s0-enterprise-tool-gateway-contract-spec.md`、`measix-s0-android-integration-contract-spec.md`、`measix-s0-control-protocol.md`
> 文档职责：固定 S0 必须跨仓库稳定的技术方向、工程原则、repo/process 边界和实施依赖；不维护 source tree、class/file、DDL、运行配置名或当前实现状态。

## 1. Repository 与 process 决议

S0 六个逻辑组件；Enterprise Portal 已在 S0.2 implementation entry 登记为同级独立实现仓库：

```text
Android Client  → topabomb/rikkahub_mcp
Control Hub     → topabomb/measix-platform-core
Runtime Relay   → topabomb/measix-platform-core
Enterprise Tool Gateway → topabomb/measix-platform-core
Admin Console   → topabomb/measix-platform-core
Enterprise Portal → measix-enterprise-portal (independent WebView SPA repository)
```

服务端 production daemon 固定为：

```text
control-hub
runtime-relay
enterprise-tool-gateway
```

Admin Console 是 static SPA，不增加独立 Node production daemon。

实际 source/package/module 结构由各实现仓库维护；architecture 只约束组件依赖方向与长期边界。

## 2. 技术方向

S0 采用轻量、可审计、可本地部署的技术基线：

```text
Backend/runtime        Go
HTTP                    net/http + lightweight routing
Executable API          OpenAPI 3.0.x + generated wire types
Control Hub persistence SQLite + versioned migrations + ORM/schema tooling
Relay usage spool       SQLite + explicit narrow persistence boundary
Admin                   Vue 3 + TypeScript + Quasar static SPA
Android                 existing Kotlin/Compose/OkHttp/kotlinx.serialization/DI/persistence stack
```

具体 minor version、package、lockfile、generated target 和 migration SQL 由实现仓库 pin。

S0 不引入 PostgreSQL、Redis、Kafka/RabbitMQ、Service Mesh、独立 API Gateway、Node SSR、第二套 Android network/DI/database framework，除非未来 architecture requirement 已经证明现有方案无法满足。

### 2.1 生产进程监管基线

S0.3 增加第三个 daemon 后，生产部署必须把三个 daemon 交给宿主原生 service manager 监管；reference profile 是 Linux `systemd` + `journald`。满足同一合同的等价 supervisor 可以替换 reference profile，但 S0 不创建第四个 MEASIX “process-manager” daemon，也不把开发用并发启动脚本或测试 harness 当作生产监管器。

最低合同：

- 每个 daemon 一个独立 service/failure domain，并提供一个 aggregate target/group 统一 start/stop/status；单个服务失败不得无条件级联重启全部服务；
- 生产安装使用已构建、可追溯 build identity 的 binary，不使用 `go run`、前端 dev server 或 `concurrently`；
- supervisor 负责开机启动、`on-failure` restart、有限 restart delay/backoff 与 start-rate limiting；永久配置/迁移错误必须可诊断且不能形成无界 crash loop；
- 正常 stop/restart 发送终止信号，daemon 停止接收新工作、在有界 grace 内 drain，再由 supervisor 在超时后强制终止；
- process `active` 只证明进程存活，不等于应用 `ready`。流量与发布判断仍以组件 readiness、desired/applied revision/hash 和产品状态为准；
- Gateway/Relay 重启后没有 Hub rehydrate 的有效 applied state 时保持 fail closed；Hub 恢复后重放完整 desired state。

具体 unit 名称、依赖表达、用户/权限、目录、timeout、exit-code 分类、打包与 runbook 归 `measix-platform-core`。S0.3 只要求单机 reference deployment 的稳定监管；容器编排、HA、leader election、跨主机调度和自研 watchdog 延后到真实需求出现后。

### 2.2 结构化日志与收集基线

三个 daemon 以 line-delimited JSON 写 stdout/stderr，由 service manager/journald 收集、轮转和保留；应用不各自维护共享滚动日志文件。每条生产日志至少包含：

```text
time
level
msg
service          control-hub | runtime-relay | enterprise-tool-gateway
buildVersion
event            stable machine-searchable event name
```

仅在事实存在时增加 `requestId`、`interactionId`、`activationId`、`deploymentId`、`managedGeneration`、`controlRevision`、`gatewayControlRevision`、`resourceId`、`gatewayToolId`、`durationMs`、`outcome`、`errorCode`。HTTP 日志使用 route template，不记录 raw URL/query。默认生产级别为 INFO，DEBUG 显式开启且不得改变脱敏规则。

禁止记录 Authorization/Cookie/Secret/credential、Enrollment/Refresh material、private endpoint、toolRef 或其 claims、完整 prompt/body、tool arguments/result、用户名/邮箱等直接身份信息。生命周期、readiness transition、control apply/reconcile、degraded/recovery 与安全拒绝只记录必要摘要和 correlation。集中式搜索/告警平台不是 S0.3 前置条件；实现仓库必须固定安全采集命令、保留策略和相应测试。

## 3. Architecture first, executable contract second

跨组件语义修改顺序：

```text
Architecture authority
→ Control Protocol（wire/state/error/security 变化时）
→ executable OpenAPI
→ canonical fixtures
→ generated wire types
→ implementation
→ executable tests/evidence
```

实现不得通过 Go struct、Vue form、Android data class 或既有数据库反向创造 wire semantic。

非语义的实现重构、package 拆分、索引优化、helper 调整不需要 architecture change。

## 4. State 与 persistence 原则

### Control Hub

- Hub 是 Identity、Draft/Release/Snapshot、operational state、desired Runtime Control、Usage/Pricing 的 durable authority；
- durable entity/revision 必须在 crash/restart 后可恢复；
- network call 不包在长 DB transaction 内；
- Release/Snapshot/SecretVersion 等被定义为 immutable 的历史不得原地重写；
- schema change 使用 versioned migration，生产不依赖 runtime AutoMigrate；
- backup/restore 必须可验证 schema revision、integrity 和关键 stable IDs。

具体 table/entity/column/index/DDL 由 `measix-platform-core` 维护，只要满足这些 invariants 与 executable contract。

### Runtime Relay

- Relay 不访问 Hub DB、不 import Hub domain/persistence；
- runtime control 由 Hub 发送完整 desired state，Relay 完整验证后原子替换；
- Relay 不持久化可重建的完整 control state/credential 副本；restart 后无有效 state 时 fail closed，等待 Hub rehydrate；
- request usage 在承诺 at-least-once 前必须进入 durable local spool；
- runtime data path 不引入 ORM、broker 或同步 Hub lookup。

具体 atomic primitive、proxy helper、spool DDL 和 process layout 由 core 实现决定。

### Enterprise Tool Gateway

- Gateway 不访问 Hub DB、不 import Hub durable domain/persistence；
- Gateway control 由 Hub 发送完整 desired state，Gateway 完整验证后原子替换；
- Gateway 不持久化可重建的完整 control/credential authority；restart 后 fail closed，等待 Hub rehydrate；
- Candidate/Published Catalog durable truth、Managed Release 与 gateway control desired truth 都在 Hub；
- search index/downstream session/toolRef 都是 applied/runtime state，不反向成为发布 authority；
- tool-level execution facts 与 Relay request facts 通过 request identity correlation，不在两个组件重复声明同一事实 owner。

具体 index/session/client/cache/telemetry 设计由 core 实现决定。

## 5. Runtime / Provider 边界

Runtime Relay 永远保持 provider-agnostic transparent L7：

```text
Client-compatible request
→ Relay admission/security/credential/routing
→ transparent HTTP transport
→ Upstream Adapter
→ actual provider
```

Relay 不包含：

```text
if provider == OpenAI/Anthropic/Google
body/schema translation
token guessing
provider SDK business semantics
```

Provider/protocol compatibility 位于 Upstream Adapter；Client-facing required profile 由 sub-stage contract 和 qualification 决定。

## 6. Admin production 边界

Admin production 形态：

```text
SPA source
→ production static artifact
→ same-origin platform ingress/Control Hub static serving
→ /api/admin/v1/*
```

Admin 不拥有 server validation/state transition，不直连 Relay internal API，不持久化 Secret plaintext，不维护平行 wire DTO。

具体 Vue component/store/composable、npm dependency 和 build command 归 `measix-platform-core`。

## 7. Android integration 边界

S0.1 阶段 `rikkahub_mcp` 是 read-only integration target：

- 可以审查现有 Model/TTS/ASR/MCP/Settings runtime reality；
- 不落 Enterprise Binding/Managed Runtime implementation；
- 不把 pre-freeze Snapshot DTO 固化到 Android；
- server-side proof 使用 deterministic Test Client。

只有有效 S0.1 Freeze 后才开始 S0.2。

S0.2 必须复用 Android 现有 Assistant/Memory/QuickMessage/MCP/chat runtime 边界完成最小 A/B/C 投影，不建立第二套 Enterprise Chat/Provider stack；Managed definition/state 与现有 Local records 物理分离。Enterprise Portal 是独立 WebView SPA，但 Android Native Host 拥有 Realm、Session、Bridge 和本地状态边界。

本地示例网页与远端 Portal 必须消费同一 Native Bridge v3 和同一状态/操作/动态/媒体产品语义，不能各自维护不同的同步或录音协议。优先复用独立 Portal 的页面与状态逻辑，差异收敛到构建资源来源及受控 Session/Feed I/O：远端使用 Hub HTTP，会话与凭据由平台/原生负责；本地静态资源由 Android 专属 HTTPS origin 加载，数据通过同一 Bridge 的类型化消息读取，不伪造生产身份，不增加通用 HTTP 代理或第二桥接入口。真实生产远端产物不得提供未认证的运行时来源切换或模拟登录旁路。

本地读取方案裁决：采用带原生 origin/发起 frame 信息及原文档回复通道的消息传输，保持身份、Feed DTO、日期、发布 revision 和先授权再判断表示是否变化的语义。废止拦截本地 GET 同时证明 fetch 发起 frame、构造 HTTP 304 的要求；不选择放宽 frame 验证或用固定 200 掩盖协议差异。依据是 Android 公开 [WebResourceRequest](https://developer.android.com/reference/android/webkit/WebResourceRequest) 不提供 fetch 发起 frame 身份，[WebResourceResponse](https://developer.android.com/reference/android/webkit/WebResourceResponse) 拒绝 3xx，而 [WebMessageListener](https://developer.android.com/reference/androidx/webkit/WebViewCompat.WebMessageListener) 提供 sourceOrigin/isMainFrame 与原请求回复代理。Bootstrap、版本、消息结果及失效语义统一见 Control Protocol §8；本地消息未变化结果不等同于原生 HTTP 304。该决定不改变产品范围或远端 HTTP 合同。

交付同一源代码的 remote/local 两份独立构建与资源清单，清单固定来源、Bridge/本地读取版本、契约摘要和资源 hash；本地所有资源随包，来源协议见 Control Protocol §8。Android 使用已验证清单集成，不在运行时把远端构建降级为本地。Core 保存可执行契约和唯一共享样例，Portal 生成消费类型并记录输入摘要；Android 消费相同契约。前端组件、HTTP 服务与真实设备验证分别报告，三个仓库的完成不能替代 Android 权限/媒体和 Stage Freeze。

本地企业配置文件只声明资料与配置，不承载另一套可执行 Portal 页面。原生宿主集成独立 Portal 的 local 资源包，删除旧内嵌 HTML/未版本化 bridge 草案的生产消费路径，不维护旧新双协议。各消费者执行同一份正反例，资源与契约使用可校验摘要固定；准备和验证本地闭环无需等待真实平台或 Gateway，但不得据此宣称生产互操作完成。

S0.3 先用 Admin + Test Client 完成 Gateway/Profile/Catalog/two-tool surface/platform+external MCP、Gateway-first/Relay-second activation 和服务端 Freeze，不以 Android 验收替代服务端合同。

S0.4 在 S0.3 frozen Snapshot v5 上完成 Model/TTS/HTTP-ASR/Direct MCP/Gateway required profile、Effective Runtime 和 failure/concurrency integration，仍复用 Android 现有 runtime/network/DI/persistence 主体。

## 8. 实施顺序

历史 `I0–I5` 仅作为工程 workstream 标签，不作为 release sequencing authority：

```text
I0 Engineering & Contracts
I1 Identity & Enrollment
I2 Managed Definition / Draft / Snapshot
I3 Runtime Relay / Publish
I4 Android Enforced Runtime
I5 Metering / Admin / Hardening
```

正式 delivery gate：

```text
S0 Core foundation
  ↓
S0.1 Managed Capability Delivery
  ↓
S0.1 Client Contract Freeze
  ↓
S0.2 Enterprise Realm & Experience Foundation
  ↓
S0.2 Snapshot v4 / Product Freeze
  ↓
S0.3 Enterprise Tool Gateway & Governed Tool Integration
  ↓
S0.3 Snapshot v5 / Gateway Freeze
  ↓
S0.4 Android Managed Runtime Integration
  ↓
S0 Final System / RC
```

S0.1 的具体 C0–C7 sequencing 只由 `measix-s0-capability-delivery-implementation-decision.md` 维护，本文不复制 checkpoint 细节。

## 9. Test / evidence 决议

测试按长期层次区分：

```text
T0 Static / Contract
T1 Unit / Domain
T2 Component Integration
T3 Cross-component Integration
T4.1 S0.1 Product/System (pre-Android)
T4.2 S0.2 Product (real Android + Portal)
T4.3 S0.3 Gateway Product (real Admin + Test Client + three daemons)
T4.4 S0.4 Android Integration (real Android full profile)
T4 Final S0 System / RC
```

原则：

- persistence/HTTP/streaming/atomicity 等 critical boundary 使用真实边界；
- deterministic Adapter 用于可重复系统 proof，但不能替代 real Adapter qualification；
- S0.1 Test Client 不能替代 final Android T4；
- critical gate 不靠 retry-until-green 或固定大 sleep；
- Freeze/RC evidence pin exact commits/build/contracts/artifacts，不使用 floating `latest`。

具体 test files、runner、CI workflow、artifact packaging 由实现仓库维护；Architecture Testing Specs 只定义必须证明的场景和 evidence content。

## 10. 过度设计约束

S0 明确不提前引入：

- multi-Hub consensus/HA；
- Relay multi-phase PREPARE/BARRIER/COMMIT state machine；
- durable Relay control-state/Secret cache；
- durable Gateway full control-state/Secret cache；
- Redis/Kafka/message broker；
- generic workflow/scheduler；
- generic path/body rewrite DSL；
- generic quota engine；
- 自研生产 process manager、通用 scheduler 或独立日志采集 daemon；
- broad provider-native matrix；
- Runtime WebSocket skeleton；
- Gateway write/destructive tool、HA、必选 vector/Router LLM 或通用 Connector skeleton；
- 空的 future navigation/module/package。

任何新增 abstraction 必须对应当前 Stage 的真实 requirement、failure mode 或可验证 maintenance benefit。

## 11. Freeze 与后续兼容

S0.1 候选固定当前资源协议；S0.2 Freeze 固定 Snapshot v4、Android/Portal product projection；S0.3 Freeze 固定 Snapshot v5、Gateway surface/Catalog/control 与 exact server executable baseline。Freeze 后：

- backward-compatible optional extension 可按 Control Protocol 兼容规则演进；
- breaking client-visible semantic 必须走显式 schema/API compatibility decision；
- S0.4 Android integration 使用 pinned S0.3 baseline，不追随 moving platform-core head；
- architecture/core/android candidate 发生相关变化时，受影响 gate 必须重新执行。
