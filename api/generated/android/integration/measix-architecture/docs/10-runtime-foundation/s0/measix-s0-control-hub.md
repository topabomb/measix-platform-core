# S0 Control Hub 架构与需求规格

> 状态：S0 Component Architecture / 组件架构基线  
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`  
> 术语/ID 权威：`../../00-platform/measix-platform-terminology-and-identifier-contract.md`  
> 跨组件契约：`measix-s0-control-protocol.md`  
> 实施决议：`measix-s0-implementation-decision.md`  
> Admin 产品：`measix-s0-admin-console-product-requirements.md`  
> S0.2 企业经验：`measix-s0-enterprise-realm-experience-contract-spec.md`
> Relay：`measix-s0-runtime-relay.md`  
> 测试：`measix-s0-control-hub-testing-spec.md`  
> 文档职责：定义 Control Hub 的职责、领域 ownership、durability/recovery/security boundary 和与其他组件的架构关系；具体 package/file、Ent schema/DDL、migration SQL、环境变量名和当前实现由 `measix-platform-core` 维护。

## 1. 正式定位

Control Hub 是 S0 的**控制面权威中心**。它负责回答：

- User/Device/Session 的 authoritative identity/state；
- 企业当前 Draft、Published Release、Managed Snapshot 与 active generation；
- Upstream/Secret 的 candidate/active operational state；
- Runtime Relay / Enterprise Tool Gateway 应执行的两份完整 desired control state；
- Publish/Apply/Security change 是否已经被所有受影响 runtime target enforce；
- Request/Semantic Usage、Pricing 与 Cost 如何归属；
- Managed Assistant/Memory Seed/Starter、Enterprise Update、Tool Gateway Profile/Catalog/Integration 如何发布、投影和版本化；
- Admin/Android 应看到的控制状态。

Control Hub **不进入** Model/TTS/ASR/通用 MCP 大流量 data path，不做 Provider-specific body translation 或通用工具代理。Enterprise Update 只提供受限 private typed read API。

## 2. Domain ownership

Control Hub 按职责拥有以下领域边界：

```text
Identity
  Deployment / User / Device / Enrollment / Session / signing state

Managed Capability
  Draft / Validation / Provider / Model / TTS / ASR / MCP / Policy
  Managed Assistant / Memory Seed / Assistant Starter
  Tool Gateway Profile / Tool Integration / Candidate + Published Tool Catalog
  Release / Snapshot / Managed State

Enterprise Update
  Draft / Publish / Withdraw / Feed revision / client projection
  private typed read projection for Gateway platform adapter

Operational Runtime
  Upstream / Secret / Runtime Binding/Route
  Activation / desired RuntimeControlState + GatewayControlState / reconciliation

Usage
  Request Usage ingest / semantic usage / pricing / cost

Platform Surface
  Discovery / Admin API / Client Control API / internal usage endpoint
  Admin static artifact hosting boundary
```

这些是**领域职责**，不是 implementation package/file 规范。具体代码 decomposition 归 core 仓库。

## 3. 核心 state 边界

### 3.1 Identity

- Deployment 是 S0 单部署 authority；
- `EnterpriseUser` 使用 role 表达 ADMIN/MEMBER，不建立平行 Admin identity；
- Device、Session、Enrollment 是独立生命周期对象；
- API identity 必须从受信任 Session/Token/Enrollment authority 推导，不信任业务 request 中任意提交的 user identity；
- Enrollment one-time/short-lived，明文 secret 只存在生成/交换生命周期，durable store 只保存不可逆验证材料；
- restrictive revoke/disable 先提交 Hub authority，再推进 Relay enforcement，不因 Relay 暂时失败回滚成 ACTIVE。

Stable ID 具体 prefix/owner 以 Terminology Contract 为准，本文不复制完整 ID 表。

### 3.2 Managed Draft

Draft 是当前唯一可编辑 Managed Capability workspace：

```text
ManagedDraft
  draft identity
  monotonic draftRevision
  typed resource/policy/binding aggregate
  audit metadata
```

要求：

- mutation 使用 optimistic concurrency；
- Save Draft 不改变 active Release/Runtime；
- caller-proposed child ID 只有经 Hub 做 type/prefix/uniqueness/reference validation 并持久化后才成为 authoritative stable identity；
- browser/raw JSON/database 不能绕过 Draft validation 直接改变 active state。

### 3.3 Release / Snapshot

Published Release 是 immutable history；Snapshot 是其中的 client-visible filtered projection。

不变量：

- Release content / generation / Snapshot 一旦发布不得原地改义；
- rollback = republish historical content as a new Release/generation；
- Snapshot 必须由同一个 canonical compiler/projection path 产生；
- Snapshot 不包含 Secret、Upstream URL、runtimeRouteId、server-only Binding/Pricing；
- Snapshot hash/ETag 语义以 Control Protocol / executable Client contract 为准；
- `managedGeneration=0` 仅表示尚无 active Release，不伪造 generation-0 Snapshot。

### 3.4 Managed State

Hub durable 维护当前 client/control authority：

```text
activeRelease
activeManagedGeneration
managedStateRevision
runtimeStatus READY | ACTIVATING | DEGRADED
current desired control revision/hash
```

四个 version sequence（managed/control/gateway-control/managed-state）的角色由 Foundation/Control Protocol 定义，不能混用。

### 3.5 Enterprise Update authority

Enterprise Update 与 Managed Draft/Release/Snapshot 分离。Hub durable 拥有 Update Draft、PUBLISHED/WITHDRAWN state、Feed revision/ETag 和 Client/Portal projection；发布/撤回不得推进 `managedGeneration`。

S0.3 由 Enterprise Tool Gateway 的 platform adapter 调 Hub 窄范围 private typed read API。该 API 只读取 PUBLISHED Feed、不复用 Admin mutation surface、不实现 MCP initialize/tools/list/tools/call，也不改变“Hub 不代理通用 runtime/tool payload”的边界。

### 3.6 Enterprise Tool Gateway authority

Hub durable 持有：

```text
ToolGatewayDefinition / ToolGatewayProfile
ToolIntegration config + Managed Secret references
Raw Candidate Catalog + drift diff
immutable Published Tool Catalog @ managedGeneration
agent-facing name/description/aliases/risk/enabled
desired GatewayControlState revision/hash + applied status
```

Hub 不执行每次 discover/invoke、不维护 downstream MCP runtime session、不处理真实 tool arguments/result。Candidate refresh 只能改变 review workspace；Published Catalog 只有新 Managed Release 才改变。

## 4. Upstream / Secret / Runtime Binding

### 4.1 Candidate 与 Active 必须分离

Upstream edit 先产生 candidate revision；只有显式 Apply、Relay ACK 后才改变 active operational revision。

```text
Edit candidate
→ Save
→ optional Test
→ Apply command
→ full desired control state
→ Relay ACK
→ active revision changes
```

Apply 失败必须保留之前 active runtime，同时保留 candidate 供修正/重试。

### 4.2 Secret

- Secret plaintext create/replace 后永不通过普通 API 回显；
- Secret version immutable；replace 创建新 version；
- candidate config 必须精确引用所选 Secret version；
- Secret replace 本身不自动改变 active Runtime；需要更新 candidate 并 Apply；
- Snapshot、Usage、Problem、普通日志不得出现 plaintext credential。

具体 encryption primitive、DB columns、key-file naming 属于 core implementation，只要满足 architecture security invariants。

### 4.3 Runtime Binding / Route

Client Definition 与 server Runtime Binding 分离：

```text
client: resourceId + client protocol/runtime metadata
server: resourceId → internal route → active upstream
```

Hub validation 必须确保 enabled Managed Resource 有唯一有效 server-side execution binding，且 method/path/transport/upstream/secret 引用闭合。

S0 不提供 generic path rewrite DSL。Provider-specific path/body/query conversion 属于 Upstream Adapter。

## 5. Validation / canonical compiler

Publish 前必须由 Hub 做 authoritative validation，至少阻止：

- ID type/prefix/duplicate/reference 错误；
- resource/provider/default/binding 引用断裂；
- enabled resource 缺 execution binding；
- binding 指向 disabled/missing/invalid Upstream；
- credential requirement 未满足；
- runtime path/method/transport 违反 S0 contract；
- required resource 缺运行必要字段；
- unsupported/unqualified protocol 被当作当前 supported profile；
- Snapshot projection 泄漏 server-only material。
- Gateway agent-facing name 重复、highlighted reference 悬空/disabled；
- downstream source schema/description/annotation 未审核或 schema 不在 supported validation profile；
- 非 READ_ONLY 或同一来源 Direct+Gateway 双路径暴露；
- Snapshot v5 surfaceHash 与 compiled Gateway surface 不一致。

Warning 只能用于允许继续但必须显式 review 的情况；warning 不能绕过 hard validation。

Snapshot Preview 与真正 Stage/Publish 必须调用同源 canonical projection，浏览器不得重建第二套 Snapshot。

## 6. Publish / Activation / desired-state control

S0 Publish 的 durable workflow：

```text
validate authoritative Draft
→ persist immutable staged Release/Snapshot + Activation intent
→ persist desired GatewayControlState + RuntimeControlState identities/revisions/hashes
→ when target Release includes Gateway, send its generation mapping in full desired state to Gateway first
→ Gateway validates + atomically applies + ACKs exact revision/hash
→ send full desired state to Relay
→ Relay validates + atomically applies + ACKs exact revision/hash
→ Hub finalizes Release ACTIVE / previous SUPERSEDED
→ activeManagedGeneration and managedStateRevision advance
→ Activation COMPLETED
```

关键不变量：

- network call 不包在长 DB transaction 内；
- Control Protocol §6 规定的 enforcement ACK 完成前不得显示 Publish success；限制性移除后的不可达 Gateway cleanup 不阻塞 finalize；
- retry 使用同一 command idempotency identity，不产生第二个 semantic command；
- Hub crash/timeout 不能靠猜测判断 Gateway/Relay 是否应用成功；必须分别读取 applied status 并 reconcile；
- unexpected newer/different Gateway/Relay state 进入 DEGRADED/operator diagnosis，不盲目覆盖未知更高 state。

目标含 Gateway 时，即使只是 Model/Assistant 变化，也须建立新 generation 的 Gateway 映射。目标移除 Gateway 时按 Control Protocol §6 先移除 Relay route，再做可重试 cleanup；cleanup 不阻塞安全移除后的 finalize。S0 不使用 PREPARE/BARRIER/COMMIT/ABORT 分布式 saga；新增/保留 Gateway 的权威模型是 Gateway-first、Relay-second 的两份完整 desired-state apply + exact status reconciliation。Gateway ACK、Relay失败时新 Catalog 保持 public-unreachable dormant，不回滚为另一条执行路径。

## 7. Security state change

Restrictive change（例如 User Disable、Device/Session Revoke）：

1. Hub 先持久化 restrictive authority，使新的 Control/refresh 行为立即拒绝；
2. 创建可恢复 Activation；
3. 编译新 full control state 下发 Relay；
4. Relay ACK 后显示 fully enforced。

Relay 不可达不能回滚 Hub restrictive state。

Permissive change（例如重新 Enable）必须避免出现“Hub 已 ACTIVE 但 Relay 仍 deny”的假成功；只有对应 Relay desired state 成功后才 finalize permissive ACTIVE。

## 8. Client/Admin/Internal surface 边界

精确 endpoint/schema 以 Control Protocol 和 executable OpenAPI 为准。Architecture 只固定 trust boundary：

```text
Discovery             public, non-sensitive
Admin API             browser admin session + CSRF / admin role
Client Control API    enterprise client bearer/session authority
Hub→Relay control     private service identity
Hub→Gateway control   private service identity
Gateway→Hub updates   private service identity / typed read only
Relay→Hub usage       private service identity
Admin static assets   public/same-origin application surface
```

要求：

- Admin/Client/Internal 不共用万能认证面；
- `/internal/*` 不接受 User/Admin credential 并且不经 public ingress；
- Client API 从 token/session principal 推导 identity；
- Enrollment 初始化七天 idle expiry；只有成功 authenticated Refresh 原子轮换 Refresh Credential 并续期，普通 API/Runtime/同步/Portal load 不单独续期；
- Admin Console 与 Admin API 默认 same-origin，不依赖 wildcard CORS；
- dangerous maintenance/bootstrap 不暴露为常规 public Admin API。

## 9. Usage / Pricing ownership

### Request Usage

Relay 提交 request-level事实；Hub durable ingest 并以 request identity 去重。事件必须保留发生时捕获的 principal/resource/upstream/generation/control revision/status/duration 等事实，不能在后续根据“当前”state重新解释历史。

### Semantic Usage

Provider/Adapter semantic usage 是可选增强：

- LEVEL_0：只有可靠 request facts；
- LEVEL_1：可靠 aggregate semantic usage；
- LEVEL_2：可稳定与具体 platform request correlation。

只有具体 connector/Adapter qualification 能证明时才可宣称 LEVEL_1/2；禁止用 byte/log heuristic 猜 token/audio 等语义量。

### Pricing / Cost

Pricing 只对可靠 meter 计算。缺失/部分 semantic source 时 cost 必须保持 UNKNOWN/PARTIAL，不伪造 0。

具体 ledger/table/index/query implementation 由 core 仓库维护。

## 10. Persistence / initialization / recovery invariants

Hub 的 durable store 必须满足：

- stable lifecycle identity 在 restart/backup/restore 后不变；
- Draft revision、Release generation、desired control revision、idempotency identity 等关键唯一/单调约束可由 durable layer 强制或可靠验证；
- immutable Release/Snapshot/SecretVersion/历史 usage 不原地重写；
- 当前完整 schema 使用唯一初始化 SQL；非当前数据库 startup fail-fast 并要求清理重建，不执行增量升级；
- backup/restore 能验证 database integrity、当前 schema identity 和 critical identity/history；
- Secret/master signing material 与普通 DB backup 的安全边界明确，credential 不因为 restore 缺失而静默当空值运行。

具体 Ent table/column/index、SQLite pragma、初始化 SQL、backup CLI command 属于 core implementation/operations 文档。

## 11. Reconciliation 与可观测性

Hub 必须能根据 persisted desired states 与 Gateway/Relay applied status 分别收敛：

```text
same revision/hash     → converged / finalize pending when appropriate
target missing/older   → replay same target desired state
all targets exact      → finalize persisted Activation
unexpected newer/hash  → DEGRADED + operator diagnosis
```

System/Admin observability 至少能表达：

- active generation / managed-state revision；
- desired vs applied Relay/Gateway control revisions/hashes；
- Relay/Gateway ready/last seen；
- latest/pending/failed Activation；
- DB/migration health；
- usage ingest lag / semantic unknown/orphan；
- Upstream/Tool Integration/Catalog drift operational state。

日志使用 stable identity/correlation，但不记录 Secret/token/prompt/body。

## 12. 明确非目标

Control Hub S0 不负责：

- Provider runtime data proxy/translation；
- per-request discover/invoke/search/schema validation/downstream MCP session；
- Android Conversation/Message 存储；
- Organization/Group/RBAC 和 per-user capability assignment；
- Agent Space/Agent Runtime/Runtime Hook；
- multi-Hub HA/consensus；
- generic scheduler/queue/quota engine；
- complex background Upstream health platform；
- SaaS billing/invoice。

## 13. Component Exit

Control Hub 合格必须由 `measix-s0-control-hub-testing-spec.md` 及对应 System Testing Spec 证明至少：

1. Draft/Release/ManagedState 不混淆；
2. Publish 所需 enforcement ACK 前不成功，移除/cleanup 按 Control Protocol §6，crash/timeout 可 reconcile；
3. candidate/active Upstream 与 Secret version 行为正确；
4. 当前 Snapshot 与后续 Gateway surface canonical、deterministic、client-safe；
5. restrictive/permissive security enforcement 不产生假 ACTIVE；
6. Request Usage durable ingest/dedupe，semantic/cost 不伪造；
7. migration/backup/restore 不破坏 critical identity/history；
8. Admin/Client/Internal trust boundary 分离；
9. Hub 不进入 Provider 或通用 Tool Runtime data path；
10. Snapshot v4 Assistant/Seed/Starter 与独立 Enterprise Update Feed deterministic、client-safe、版本互不串扰；
11. Tool Gateway Profile/Integration/Candidate/Published Catalog ownership、Gateway control 与 private Update read API 正确；
12. implementation repository 可以自由重构内部代码，而无需复制或改变上述 architecture invariants。
