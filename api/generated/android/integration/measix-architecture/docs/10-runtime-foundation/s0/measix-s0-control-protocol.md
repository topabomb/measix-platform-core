# S0 跨组件 Control Contract

> 状态：S0 Cross-component Authority / 跨组件权威规格  
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`
> S0.1 产品范围：`measix-s0-capability-delivery-contract-spec.md`
> S0.2 企业域与经验入口：`measix-s0-enterprise-realm-experience-contract-spec.md`
> S0.3 Gateway 入口：`measix-s0-enterprise-tool-gateway-contract-spec.md`
> S0.4 Android 入口：`measix-s0-android-integration-contract-spec.md`
> 术语/ID 权威：`../../00-platform/measix-platform-terminology-and-identifier-contract.md`  
> 文档职责：定义 Control Hub、Runtime Relay、Enterprise Tool Gateway、Admin Console、Android/Test Client 之间的 wire、权威状态、Publish/Apply、Snapshot、Runtime request、Usage、错误和故障恢复。Executable schema 位于 `measix-platform-core/api/`，必须与本文一致。

## 1. 协议目标与非目标

S0 必须保证：

1. Publish 成功后旧 `managedGeneration` 不能开始新的 Managed Runtime request；
2. Client 每次新顶层 Managed interaction 前能确定是否需要同步；
3. Relay 在向 Upstream 写 body 前完成 Auth / Principal / Generation / Resource / Route 检查；
4. Admin 不会看到“Publish 成功但 Gateway/Relay 仍执行旧状态”；
5. Hub/Gateway/Relay crash、timeout 后可通过各自 desired/applied state 收敛；
6. 一次 interaction 不静默切换 generation；
7. Request Usage 在 Hub 短时不可用时不静默丢失；
8. S0.1 Client Snapshot 能完整表达首个 required Managed Capability profile；
9. MEASIX 尚未发布，只支持当前 Snapshot v4；Gateway v5 为后续目标。旧原型版本、兼容和升级不构成当前要求，唯一规则见 §10.10.1。

S0 不要求：Push、Control SSE、Device heartbeat、Runtime WebSocket、多 Relay 共识、多 generation 灰度、generic path rewrite、Provider-specific body translation。

## 2. Contract artifacts 与 API 面

Executable contract 至少分五份：

```text
measix-platform-core/api/
├── admin/admin.openapi.yaml
├── client/client-control.openapi.yaml
├── internal/relay-control.openapi.yaml
├── internal/gateway-control.openapi.yaml
├── internal/usage-ingest.openapi.yaml
└── fixtures/
```

统一 external origin：

```text
/.well-known/measix      Discovery
/api/admin/v1/*          Admin Console → Control Hub
/api/client/v1/*         Android/Test Client → Control Hub
/runtime/v1/*            Client → Runtime Relay
/internal/v1/*           Hub ↔ Relay/Gateway private only
```

### 2.1 JSON / compatibility

- OpenAPI 3.0.x（具体版本由 core pin）；JSON UTF-8；平台 HTTP 字段 `camelCase`；时间 UTC RFC3339；外部协议及明确冻结的业务 tool schema 字段不改名，例外见 §10.16；
- response consumer 忽略未知 optional field；
- request 未声明 field 默认 `400 invalid_request`；
- 同一当前 API `/v1` 允许新增客户端可忽略的 optional response field/endpoint；
- 删除 field、改义、optional→required 属于 breaking change；
- enum extension 只有 consumer 已有 unknown-value compatibility strategy 时才可原地增加，否则视为 breaking；
- Secret/token/prompt/body 不进入 Problem、普通日志或 Usage。

### 2.2 当前协议与阶段证据

MEASIX 尚未发布，按 §10.10.1 仅支持当前 Snapshot v4。S0.1 的资源能力门禁仍须执行，但旧原型 v1 的声明不构成发布或兼容义务。所有新候选记录实际源码、OpenAPI、fixture 和当前 schemaVersion；不得把旧报告视为当前验收。

### 2.3 S0.2 Snapshot Freeze

S0.2 Exit 固定：

```text
architectureCommit
platformCoreCommit
androidCommit
portalBuildIdentity
clientControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion=2
S0.2 scenario/evidence manifest
```

S0.3 Gateway 在该 pinned baseline 上增加 v3，不原地修改 v2。

### 2.4 S0.3 Snapshot / Gateway Freeze

S0.3 Exit 固定：

```text
architectureCommit
platformCoreCommit
gatewayBuildIdentity
adminBuildIdentity
clientControlOpenApiHash
gatewayControlOpenApiHash
canonicalFixtureHash
evaluationCatalogHash
snapshotSchemaVersion=3
S0.3 scenario/evidence manifest
```

S0.4 Android 使用该 pinned baseline，不追随 floating implementation head。当前 Gateway 修订为 Snapshot v5，在 v4 用户配置准入之上增加 Gateway logical resource 与 expected surfaceHash，不复制完整 Tool Definition；历史编号见 §10.10.1。

### 2.5 Pagination

所有 Admin list：

```http
GET ...?limit=50&cursor=<opaque>
```

`limit` 默认 50，最大 200；response：

```json
{"items":[],"nextCursor":"optional-opaque"}
```

### 2.6 Admin Idempotency / async command

跨 Relay side effect 的 Admin command 必须携带：

```http
Idempotency-Key: idem_<uuidv4>
```

适用：Publish、Republish、Upstream/Gateway Integration Apply、Catalog Refresh、User Disable/Enable、Device Revoke。

同 key + 同 normalized request → 返回同一 Activation/最终结果；同 key + 不同 request → `409 idempotency_conflict`。

接受成功：

```http
202 Accepted
Location: /api/admin/v1/activations/{activationId}
```

`202` 只代表 command 被 Hub 接受；只有 Activation `COMPLETED` 才代表 §6 所需 enforcement ACK 已取得并由 Hub finalize。移除 Gateway 后不可达旧状态的异步 cleanup 不属于阻塞 finalize 的 ACK。

## 3. 权威状态与版本

### Control Hub

```text
ManagedState
  activeReleaseId?
  activeManagedGeneration
  managedStateRevision
  runtimeStatus READY|ACTIVATING|DEGRADED
```

### Runtime Relay

```text
CurrentRuntimeControl
  controlRevision
  bundleHash
  activeManagedGeneration
  authKeySet
  principalState
  resourceRouteIndex
  routeIndex
  upstreamIndex
  operationalLimits
```

### Enterprise Tool Gateway

```text
CurrentGatewayControl
  gatewayControlRevision
  bundleHash
  deploymentId
  surfacesByManagedGeneration
  publishedCatalogsByManagedGeneration
  integrationIndex
  platformToolPolicy
  toolRefVerificationMaterial
  operationalLimits
```

### Client

```text
BindingState
  UNBOUND | ENROLLING | BOUND | REVOKED

AppliedManagedState
  appliedManagedGeneration
  snapshotHash
  schemaVersion
  releaseId

ManagedRuntimeState
  UNKNOWN | READY | SYNC_REQUIRED | SYNCING |
  SYNC_FAILED | CONTROL_UNAVAILABLE
```

版本语义：

```text
managedGeneration      Release/Snapshot 单调版本
controlRevision        Hub→Relay 完整运行控制版本
gatewayControlRevision Hub→Gateway 完整运行控制版本
managedStateRevision   Client control state 变化序号
```

四者都不是 entity ID。`managedGeneration=0` 表示 Deployment 尚无 active Release。

## 4. Identity / token contract

ID namespace 以 Terminology Contract 为权威。

Owner：

```text
Hub      activationId act_*
Android  interactionId int_*
Relay    requestId req_*
Admin    Idempotency-Key idem_*
```

### 4.1 Android Access Token

```text
alg = EdDSA / Ed25519
TTL default <= 10 min; configured maximum <= 10 min
```

Claims 至少：

```text
iss = deploymentId
sub = userId
deploymentId
deviceId
sessionId
aud includes client/runtime
iat
exp
```

Relay 只接收 Hub 下发的 public JWK；Hub private signing key 不进入 Relay/Client。

### 4.2 Refresh Credential

- >=256-bit opaque random secret；
- Hub 只存 digest + sessionId + expiry；
- Android Keystore-backed；
- Session idle expiry 为服务端权威七天滚动窗口；Refresh Credential 自身 expiry 不得越过对应 Session authority；
- 成功 authenticated Refresh 原子轮换 refresh credential、签发新 access token 并把 session idle expiry 续至 `now + 7 days`；
- logout/revoke/expiry 后失效。

### 4.3 Internal service credentials

Hub↔Relay、Hub↔Gateway、Relay↔Gateway credential 彼此和 User/Admin credential 分离；same-host 可使用受限 loopback/private listener，cross-host 必须 TLS；internal listener 不经 public ingress。

Relay→Gateway request 必须先移除 client-supplied internal headers，再注入完整性受保护的 Principal envelope。Envelope 至少包含 `deploymentId/userId/deviceId/sessionId/interactionId/managedGeneration/requestId/issuedAt/expiresAt`，并绑定 Gateway audience。Gateway 不接受 body/header 中客户端自报的同名 identity。

## 5. Publish = activate + enforce

```text
Save Draft ≠ active
Validate   ≠ active
Publish N  = activate Release N + enforce N
```

Rollback 通过历史内容 Republish 产生新 generation；generation 不倒退。

S0 不存在 `minimumAcceptedManagedGeneration` 或 grace window。

## 6. Publish / Gateway + Runtime Control desired-state apply

流程：

```text
Hub validate Draft
→ create immutable STAGED Release/Snapshot N
→ persist Activation + runtimeStatus=ACTIVATING
→ when target Release includes Gateway: compile GatewayControlState revision G
→ PUT /internal/v1/gateway-control/state
→ Gateway fully validates + atomic swap + ACK G/hash
→ compile RuntimeControlState revision R
→ PUT /internal/v1/control/state
→ Relay fully validates + atomic swap
→ ACK R + bundleHash
→ Hub finalize Release ACTIVE
→ previous SUPERSEDED
→ activeManagedGeneration=N
→ managedStateRevision++
→ runtimeStatus=READY
→ Admin success
```

Relay 不维护 PREPARE/BARRIER/COMMIT 状态机。

目标含 Gateway 时，即使只有 Model/Assistant 改变、surface/catalog bytes 不变，也必须先建立 N 的 Gateway 映射并取得 exact ACK。Gateway apply 失败时不得切 Relay；Gateway ACK 后 Relay apply 失败时，新 Catalog 在 Gateway 中是 dormant/unreachable because public Relay has not admitted generation N。Hub 通过两份 exact status reconcile 同一 Activation，不创建重复 Release/generation。

目标移除 Gateway 时，保持 Relay-current 的目录直至 Relay 移除 route 并 ACK，再清理 Gateway 不可达目录；清理是可重试的后续 control 操作，不阻塞已安全移除 route 的 Release finalize，也不得恢复 public route。Gateway 不可达不妨碍这种限制性移除。当前和目标 Release 均无 Gateway 时才完全不涉及 Gateway。

### 6.1 RuntimeControlState

```text
controlRevision
bundleHash
activeManagedGeneration
deploymentId
authKeys[]
principalState
  disabledUserIds[]
  revokedDeviceIds[]
  revokedSessionIds[]
resourceRoutes[]        # resourceId → runtimeRouteId
routes[]
upstreams[]
gatewayTargets[]
operationalLimits
```

`RuntimeRouteSpec`：

```text
runtimeRouteId
targetKind             UPSTREAM | ENTERPRISE_TOOL_GATEWAY
upstreamId?            required when UPSTREAM
gatewayTargetId?       required when ENTERPRISE_TOOL_GATEWAY, server-only
allowedMethods[]
allowedPathPrefixes[]
transportPolicy
  HTTP_REQUEST_RESPONSE
  HTTP_STREAMING_SSE
  HTTP_BINARY_STREAM
  HTTP_MULTIPART
timeoutPolicy
```

无 `pathRewrite`。

`RuntimeUpstreamSpec`：

```text
upstreamId
baseUrl
transportCapabilities[]
enabled
auth resolved runtime-only material
```

Auth runtime form：

```text
NONE
BEARER { token }
STATIC_HEADER { headerName, value }
BASIC { username, password }
```

Resolved plaintext 只经 internal secure boundary 进入 Relay memory，不写 status/spool/log。

`GatewayTargetSpec` 至少包含 private service base URL、resolved Relay→Gateway service credential、Principal envelope verification material reference 与 allowed MCP path。它只进入 Relay control，不进入 Snapshot/Client/status/log。

### 6.2 GatewayControlState

```text
gatewayControlRevision
bundleHash
deploymentId
runtimeGenerations[]       # at most Relay-current + next-staged during activation
surfaces[]
  managedGeneration
  toolGatewayId
  surfaceVersion
  discoverToolDefinition
  invokeToolDefinition
  surfaceHash
catalogs[]
  managedGeneration
  catalogHash
  publishedGatewayTools[]
integrations[]
  toolIntegrationId
  endpoint
  protocol = MCP_STREAMABLE_HTTP
  resolved runtime credential
  timeout/limits
  enabled
platformToolPolicies[]
toolRefSigning/verificationMaterial
operationalLimits
```

`runtimeGenerations[]` 不是 grace-window authority；它只支持 Gateway-first/Relay-second handoff，且必须与随附 surface/catalog 闭合。Gateway 按受信 Principal envelope 精确选 generation，不做 latest/fallback。Relay 切换后旧 generation 的新请求仍由 Relay 428；Hub finalize 后推进 Gateway control revision 并清理不可达旧 generation。

Gateway 不访问 Hub DB；完整验证后原子替换；restart 无 state 时 fail closed；Hub rehydrate。Credential/private endpoint 不进入 status/log/client。

Published Gateway Tool 的 source 为互斥联合：`sourceKind=DOWNSTREAM_MCP` 必须且只能引用 `toolIntegrationId`；`sourceKind=PLATFORM` 必须且只能引用受 allowlist 约束的 `platformToolName`。平台工具不伪装成 downstream MCP Integration。两者共用 `gatewayToolId`、已审核 source schema、agent-facing metadata 与 risk/enabled 合同。

### 6.3 Apply idempotency

| incoming | behavior |
|---|---|
| revision > current | validate + atomic apply |
| same revision + same hash | idempotent ACK |
| same revision + different hash | 409 `control_revision_hash_conflict` |
| lower revision | 409 `stale_control_revision` |
| invalid bundle | 422 `invalid_runtime_control`, old state unchanged |

Gateway revision 使用同一 matrix，但错误码为 `stale_gateway_control_revision`、`gateway_control_revision_hash_conflict`、`invalid_gateway_control`。

### 6.4 Status / reconciliation

```http
GET /internal/v1/control/status
```

returns at least：

```text
ready
appliedControlRevision
bundleHash
activeManagedGeneration
startedAt
```

Hub reconciliation compares desired/appplied revision + hash and either finalizes or replays same desired state.

Gateway 对应 private status 返回 `ready/appliedGatewayControlRevision/bundleHash/appliedCatalogHashes/startedAt`。目标 Release 含 Gateway 的 Activation 只有在 Gateway 与 Relay 两个 desired/applied revision+hash 都 exact match 后才能 finalize；Gateway 移除与后续 cleanup 按 §6 的限制性移除规则处理。

## 7. Runtime Operational / Security changes

### 7.1 Upstream operational apply

```text
Save candidate/configRevision
→ Admin :apply
→ controlRevision++
→ Relay atomic apply/ACK
→ activeConfigRevision = candidate revision
```

失败时 old active config 保留。

### 7.2 Restrictive security change

User Disable / Device Revoke / Session revoke：

```text
Hub commits restrictive state first
→ Client API immediately rejects
→ Activation SECURITY_CHANGE
→ controlRevision++
→ Relay apply/ACK
```

Relay pending 期间 Admin 明确显示 enforcement pending；Hub 不回滚 revoke。

### 7.3 Permissive enable

User Enable 使用 Relay-first completion；Relay ACK 后 Hub 才 finalize ACTIVE。

## 8. Discovery / Enrollment / Session / Bootstrap

### 原生接入资料与来源边界

扫码和粘贴输入同一种 UTF-8 JSON 接入资料，不是 Portal URL、裸 Enrollment code 或完整配置文件。解析器只去除首尾空白，限制原文最多 2048 UTF-8 bytes，拒绝重复键、未知字段、错误类型、未知版本及不完整对象；不执行其中内容、不跟随资料指定的任意回调。接入凭据不进入网页、日志、崩溃详情或普通备份。Admin 复制内容与 QR 编码必须为同一完整资料，显示一次后按原 Enrollment 生命周期清理。

资料 `formatVersion=1` 与 HTTP protocolVersion、Snapshot schemaVersion、配置文件 formatVersion 各自独立。以 `kind` 区分两种互斥格式，所有列出的字段必填：

接入资料的 expiresAt 使用明确的 UTC 输入子集：日期必须真实有效，小时 00–23、分秒 00–59；小数秒可省略，存在时为 1–9 位。接收端允许 T/t、Z/z 或 +00:00，拒绝其他偏移（含 -00:00）、闰秒、超过 9 位精度，不得通过截断/四舍五入接受原本无效的值。生成端统一输出大写 T/Z，最多纳秒精度。+00:00 是 UTC 的合法表示，不能作为“非 UTC”反例。此输入规则只用于原生扫码/粘贴资料；Portal context、Bridge 状态及服务响应继续输出大写 T/Z 的 UTC 时间，不能因为输入归一化增加其他 wire aliases。

| kind | 其余字段 | 接收与验证 |
|---|---|---|
| `PLATFORM_ENROLLMENT` | `platformUrl,code,expiresAt` | platformUrl 最多 1024 字符，为部署公共 HTTPS origin，无 userinfo/query/fragment/path（根 `/` 可归一化）；code 为 1..128 字符不透明短期凭据；expiresAt 为 RFC3339 UTC 时间，仅供预检查，服务端仍是到期/消费权威 |
| `LOCAL_EXAMPLE_ENROLLMENT` | `sourceNamespace,deploymentId,code,expiresAt` | sourceNamespace 为应用已安装并明确标为本地示例的来源 ID；其余身份和 code 由该来源校验；无 platformUrl，不能发往网络 Enrollment |

平台资料来源于当前 Admin 的部署公共 origin（不是 Hub/Relay 内网地址，也不接受 QR 自称可信）；Android 展示解析后的目标 origin，用户明确接入后从该 origin 执行 Discovery，核对产品、协议及受支持 schema，再向同一 origin 的固定 Enrollment endpoint 交换 code。禁用跨 origin 重定向，code 不进入 Discovery URL；Discovery 中的 API base 仍限定同源 path。无服务端返回前不能据二维码显示名或 code 直接发布 BOUND。仅显式开发/测试配置可允许 loopback HTTP；发行版不能把私网或任意 HTTP 当作该例外。

本地资料的 sourceNamespace/deploymentId/code 均为 1..128 字符不透明标识，不允许通过资料动态注册来源、导入脚本或打开网络地址。示例一键接入也生成同一资料并经过同一解析/验证/提交。未知来源、到期、已消费、缺配置、坏配置分别处理：前者不建立绑定；身份已成功而配置暂未就绪时保留明确的已接入/待配置状态，不能假报可执行。

本地与平台身份使用互不相交的 source namespace；资源引用、偏好、运行数据及恢复按来源/Deployment/主体隔离，平台 wire ID 本身不改写。由本地切为真实平台必须重新验证和接入，不能因 deploymentId/userId 字符串相同继承示例授权、凭据或数据。

“导入本地企业配置”使用原生系统文件选择器，接收包含定义、策略和私有 binding 的独立版本化文件；不作为扫码/粘贴接入资料，也不经 Portal。大小/格式/引用/URL/binding 完整校验后原子发布，秘密由原生受保护 owner 保存；换来源/Deployment/主体先退出再接入，同主体更新保留归属。文件格式及模板归 Android 实现仓库，不能把私有绑定加入平台 client-safe Snapshot。

### Discovery

```http
GET /.well-known/measix
```

```text
product = MEASIX_AGENT_PLATFORM
protocolVersion = 1
deploymentId
deploymentName
clientApiBase
runtimeApiBase
supportedSnapshotSchemaVersions
```

API base 是同一 `platformUrl` 下的 path，不泄露内部地址。

### Enrollment

```http
POST /api/client/v1/enrollments/exchange
```

Request：

```text
code
installationId ins_*
deviceName
appVersion
platform=ANDROID
```

Response `201`：

```text
deploymentId
userId
deviceId
sessionId
accessToken
accessTokenExpiresAt
refreshToken
refreshExpiresAt
sessionIdleExpiresAt
```

Enrollment code single-use、短时有效，Hub 只存 digest。用户必须处于 ACTIVE。

同一 installation 重新入域仍必须出示新的 Enrollment code，installationId 本身不是凭据。若已绑定同一用户且 Device 仍 ACTIVE，保留稳定 Device ID，原子撤销该 Device 旧 Session、清除其 refresh recovery 并创建新 Session；不得因已存在 installation 而永久阻止七天到期/丢凭据后的恢复。绑定其他用户返回冲突，REVOKED Device 不被 Enrollment 隐式复活；均不消耗 code。客户端换账号或重建安装身份须走明确的新 Enrollment，不靠提交他人的 installationId 接管或注销他人设备。

禁用用户/撤销设备的 deny-first 事务同时撤销关联 Session 并清除恢复材料。重新启用用户只解除 User deny，不恢复历史 Session。Android Logout 先在 Hub 持久撤销；Hub reconciler 将尚未进入已提交 descriptor 的 Android Session revoke 作为 SECURITY_CHANGE，以新的 control revision 自动收敛至 Relay（包括丢响应、重启和重新 Enrollment）。不另设消息队列；Session 撤销事实就是 durable intent。自动投影保留原 Session 所属 user 的归属并标明 SESSION_REVOKE 操作，不伪装成管理员操作。已有 Relay Access Token 在该收紧 ACK 或其短期 expiry 前可能仍被接受，不能把 Hub Logout 的 204 称作 Relay 即刻 ACK；断网时 Hub 不恢复凭据，恢复连接后继续收敛。恢复已提交 revision 必须使用其持久 descriptor，不能在旧 revision 内重新编译可变 principal state。

### Refresh / Logout

```text
POST /api/client/v1/sessions/refresh
POST /api/client/v1/sessions/logout
```

Refresh 成功返回并原子提交：

```text
accessToken
accessTokenExpiresAt
refreshToken
refreshExpiresAt
sessionIdleExpiresAt
```

Refresh 必须携带 `Idempotency-Key: idem_*`，作用域为该 Enterprise Session 的一次凭据轮换。客户端对每个 Session single-flight，在发请求前安全持久化当前 Refresh Credential 与该 key；只有完整响应安全原子落盘后，才删除 pending key 并允许下一次轮换。超时/断网重试必须使用相同 credential 与 key，不能另建 key。

服务端在同一事务内验证 Session/User/Device、轮换 credential、续期并保存结果。每个 Session 只保留最近一次成功轮换的恢复槽，成功提交后两分钟内，旧 credential + 相同 key 返回同一成功结果（包括 token 和时间），不再次签发、不二次续期；进程重启不丢失此恢复能力。该恢复槽不是第二个有效 credential，不能用于普通认证或新的轮换。

旧 credential + 不同 key 返回 `409 refresh_conflict`，不自动撤销合法 Session，避免并发请求造成恶意注销；相同 key 用于新 credential 也返回该错误。超出恢复窗口的旧 credential 返回 `401 invalid_credential`，客户端须重新 Enrollment，不能靠无界 grace window 恢复。恢复前仍验证 Session 未过期且 User/Device/Session 未被禁用/撤销，失败不得返回缓存 credential。恢复材料必须加密保存，过期不再可读，并在后续清理周期删除；下一次轮换覆盖旧槽。

Logout 使用当前 credential，或仍在两分钟恢复窗口内的上一 credential，幂等撤销整个 Session 并清除恢复槽；返回 204，未知 credential 也返回 204。Android credential 不得注销 Admin Web Session，反之亦然。Logout 不需要 Idempotency-Key。

Android Enterprise Session 使用七天滚动闲置时限；服务端时钟是唯一权威。Enrollment 成功时创建 Session 并令 `sessionIdleExpiresAt = now + 7 days`；有效 Session 的 Refresh 成功时原子轮换 Refresh Credential，并重新令 `sessionIdleExpiresAt = now + 7 days`。其他 Client API、Snapshot sync、Portal page load 和 Runtime request 均不单独续期。

Android 只在前台恢复或用户操作需要有效 Access Token 时 refresh，不允许用纯后台 heartbeat 无限保活。超过 `sessionIdleExpiresAt` 后 Refresh 返回 `session_expired`；Device revoke、User disable、Disconnect/Logout 可提前终止。Bootstrap/Refresh response 必须返回 `sessionIdleExpiresAt`；Access Token 可以更短，Portal Web Session 不能接触或替代 Refresh Credential。

### Bootstrap

```http
GET /api/client/v1/bootstrap
```

返回 deployment/user/device/session、supported snapshot schemas 和 ManagedState。

### S0.2 Session wire 冻结门槛

Refresh rotation 及以下 Portal 交互仍须 OpenAPI、持久化并发/丢响应/过期/撤销测试以及 Android 安全落盘证据共同证明；文档明确不代表阶段验收完成。

### Portal Web Session

本节 HTTP grant/Cookie/CSRF 仅用于真实平台 Portal。本地示例可在专属受信 origin 承载随包网页，但使用独立本地 document/session owner，不伪造平台 token、grant、Cookie 或生产认证结果。两种来源共用下面的 Native Bridge v3 与同一组产品行为；本地网页与远端网页的打包、资源加载和 Feed I/O 适配由实现仓库负责，不在生产远端 Portal 内放置可绕过认证的“模拟登录”开关。

S0.2 Portal 使用部署配置批准的单一 platform HTTPS origin，静态入口为 `/portal/`；不接受客户端自选 origin 或重定向地址。仅本地开发/测试允许 loopback HTTP。Hub 不从 Host/Forwarded 请求头推断批准 origin。

1. Android 以有效 Client Access Token 调用 `POST /api/client/v1/portal/grants`，无 request body。Hub 返回 `201 {exchangeUrl, ticket, expiresAt}`，`exchangeUrl` 固定为批准 origin 的 `/portal/session/exchange`。ticket 为不可预测的一次性凭据，最多 60 秒有效，绑定 Deployment/User/Device/母 Enterprise Session/origin；服务端只保存摘要。
2. Android 使用 WebView 原生 POST 向 exchangeUrl 提交 `application/x-www-form-urlencoded` 的 `ticket`。ticket 不进入 URL、历史、日志或 JavaScript；禁止以 access/refresh token 替代。Hub 原子消费 ticket 后返回 `303 Location: /portal/`，设置 `measix_portal_session` Cookie：Secure、HttpOnly、SameSite=Strict、Path=/、无 Domain。Web Session 最多 10 分钟且不晚于母 Session idle expiry。兑换丢响应时重新申请 grant，不重放 ticket；不同并发兑换最多一个成功。若 Origin 存在必须精确匹配批准 origin，cross-site fetch 拒绝；无 Origin 仅为原生 POST 交付保留。
3. Cookie 只授权 Portal session 查询/关闭和公开 Enterprise Update Feed 读取，不授权 Admin、Client Bootstrap/Snapshot、Runtime 或 grant 签发。每次请求校验母 Session、User、Device 仍有效；Refresh 轮换不延长已发出的 Web Session，Portal 操作从不续期母 Session。
4. `GET /api/portal/v1/session` 返回 `{deploymentId,userId,deviceId,sessionId,enterpriseName,userDisplayName,expiresAt,sessionIdleExpiresAt,csrfToken}`。其中 sessionId 为母 Enterprise Session，用于客户端缓存隔离；不返回任何 Android credential。所有 session/grant/exchange 响应为 `Cache-Control: no-store`。Portal Feed 复用 §10.16 的路径、query、DTO 和 ETag 语义，但只在该两个只读 endpoint 接受 Portal Cookie。
5. `DELETE /api/portal/v1/session` 必须同时具有有效 Cookie、精确 Origin 和 `X-CSRF-Token`；成功返回 204 并撤销 Web Session、清除 Cookie，不撤销母 Session。缺少或失效认证返回 401；origin/CSRF 拒绝返回 403，错误响应不泄露凭据。ticket 过期/已消费一律返回 401。Hub 重启不恢复被消费 ticket 的使用权。
6. Portal 在 401、本地到期、宿主 realm/session 变更或 pagehide 时清除内存和当前会话缓存、取消在途请求与桥接调用；不能在失效后保留可见企业数据。网络不可用且已验证的 Web Session 尚未到期时可显示其缓存，明确标记 stale；成功 Feed 刷新替换缓存并移除撤回项。read marker 仅保存本设备、当前 Deployment/User/Session 范围，不构成回执。

### 本地工作台只读来源 v2

本地随包与远端构建在构建时固定来源，不从 URL、localStorage 或认证失败选择来源。随包入口固定在宿主控制的虚拟 HTTPS origin `https://local.measix.invalid/portal/`；请求拦截只加载已验证的静态资源，不启动网络监听服务，不请求 DNS/远端资源。本地 context/Feed 数据统一使用下文 Native Bridge v3，不再使用 `/_measix/portal/v1/*` GET 或 document header，不保留双读路径。生产 Hub 不提供本地数据接口或模拟认证；远端 Session/Feed 的现有 HTTP、Cookie、CSRF、ETag/304 协议保持不变。

| 本地专用 method | params → result |
|---|---|
| `getLocalContext` | `{}` → `{formatVersion:1,kind:"LOCAL_EXAMPLE",sourceNamespace,deploymentId,userId,sessionId,documentId,enterpriseName,userDisplayName,expiresAt,sessionIdleExpiresAt}`；沿用 context DTO，不含 Cookie/CSRF/token。身份标识非空且最多 128 字符，名称最多 256 字符，时间为 RFC3339 UTC；documentId 必须与请求及原生文档一致 |
| `listLocalUpdates` | `{startDate?,endDate?,limit?,ifNoneMatch?}` → `{kind:"modified",etag,feed}` 或 `{kind:"notModified",etag}`，两个分支互斥；feed 严格复用 §10.16 的 Client Feed DTO。日期、排序、默认 limit 10/最大 20、截断与时区规则不变，Portal 显式请求 20；ifNoneMatch/etag 为非空不透明字符串、最多 256 字符，由消费者原样保存和比较 |
| `getLocalUpdate` | `{enterpriseUpdateId}` → §10.16 的 Client Feed detail DTO；标识沿用其格式约束，未发布、撤回或不存在返回 `enterprise_update_not_found` |

三个方法是本地读取 v2 的必需能力，getStatus 的 capabilities 在本地声明三者、在远端不声明；页面可直接以 getLocalContext 建立本地显示上下文，无须等待状态查询来协商协议。远端宿主拒绝这些方法为 `unsupported_method`，不代理任意 URL/HTTP。params 不接受未知字段或显式 null；参数无效返回 `invalid_request`，当前网页授权过期返回 `session_expired`，仍属当前文档但来源不再可用/被拒绝分别返回 `source_unavailable`/`source_forbidden`。跨 origin/frame、旧文档或无法安全绑定的消息直接丢弃，不向当前新文档投递错误。错误使用下文统一 error envelope，不把 HTTP status 或 Problem 套在消息结果上，不泄漏内部诊断。

每次读取先检查原生 origin、发起消息的顶层 frame、当前 documentId、来源/主体及母 Session，再在一致视图中读数据并判断 ETag。仅当有效 ifNoneMatch 与本次查询表示的 etag 完全相同时返回 notModified；其中禁止携带 feed。客户端只有在当前文档/主体及规范化查询下持有相同 etag 的完整缓存时才能接受；无缓存或 etag 不匹配视为协议错误并显示重试，不假报空列表。context/detail 不缓存，列表只保留当前页面内存缓存且每次重读复验授权；不得借缓存续期或绕过退出/撤销。这里不构造 HTTP 304，也不宣称 WebResourceRequest 能证明 fetch 的发起 frame。

本地窗口销毁由 Bridge close/原生退出 owner 完成，不另造 HTTP logout 或浏览器可写状态接口。网页可将本地与平台响应投影为显示模型，但必须保存来源/母 Session/文档身份并隔离缓存；本地不能冒用真实平台身份。页面替换、切域、退出、失效时清除页面数据/缓存并拒绝旧 documentId 和迟到响应；session_expired/source_forbidden 进入统一网页失效清理，不能降为普通离线错误，也不能据此断言母 Session 一定失效。查询更换取消原读取，超时不自动重放；三个读取方法复用同一个 Bridge 请求、取消与页面 owner。浏览器替身只用于验证协议，不随生产产物提供。

本地动态具有独立的发布状态、publishedAt、企业时区和 Feed revision。初始示例/私有导入文件可携带动态初值，但应用后动态的发布、撤回和日期读取归 Feed owner；只改变动态不得推进配置 generation，也不得使同 generation 的有效配置产生冲突。存储可复用原生已有事务载体，不要求另起数据库或服务。对 Portal 的字段投影、日期边界、truncated 与 ETag 严格复用 §10.16；认证必须先于 notModified 判断。导入的 HTML 字符串不得替代已验证资源包并取得同 origin 的原生权限。

### Portal Native Bridge v3

Bridge v3 与本地读取 v2 是本次裁决后的唯一目标协议。v3 因文档绑定及响应传输变化替代 v2；不兼容接收 v2、旧 CustomEvent 或未版本化草案，不运行时降级。本地/远端原生操作共同升级 v3；localReadVersion=2 只标识本地消息读协议，不改变 enrollment/context 的 formatVersion=1 或平台 HTTP API 版本。构建清单必须记录目标版本及对应契约摘要；协议定义不表示资源包、可执行 schema 或 Android 已同步完成。

每次批准顶层文档创建时，原生在 Portal 脚本运行前提供 `window.MeasixPortalDocument={bridgeVersion:3,documentId}`，documentId 为至少 128 bit 随机性的不可预测标识，字符串非空且最多 128 字符；在原生注册表绑定本次文档、来源/主体、母 Session 和期限。网页授权最多十分钟且不超过母 Session，重复读取 context 不续期。bootstrap 只含公开关联值，不含凭据；原生不能以其存在代替每次授权。不能可靠提供文档启动绑定或带真实来源/frame 信息的消息能力时，明确宿主不可用，不降级到不验证 frame 的接口。

Portal 仅调用宿主提供的 `MeasixHost.postMessage(JSON)`，请求为 `{bridgeVersion:3,documentId,requestId,method,params}`；requestId 非空、最多 128 字符且当前文档内唯一，params 必须是对象。信封不接受未知字段。宿主以 WebMessageListener 提供的 sourceOrigin/isMainFrame 验证精确批准 origin、发起消息的顶层 frame，并复核原生注册表中的 documentId、Enterprise Realm/主体/Session，以及方法和参数；页面传入的 origin/主体不能作为身份凭据。不得为 iframe、任意外链或模型生成 HTML 提供企业权限。静态资源拦截与消息来源验证分别承担职责，不以 Referer、CSP 或文档标识替代原生 frame 判断。

宿主保存该请求的 JavaScriptReplyProxy，完成时再次复核原文档授权，只通过该 proxy 向原消息对象回复 JSON 字符串。Portal 在发送首条请求前，由唯一 Bridge owner 设置 `MeasixHost.onmessage`，解析 event.data；响应为 `{bridgeVersion:3,documentId,requestId,result}` 或 `{bridgeVersion:3,documentId,requestId,error:{code,message}}`，result/error 严格互斥，不接受未知字段，版本/文档/请求及方法结果类型均须匹配。错误 code 必须保留到页面状态处理，不只转为文本。禁止异步完成后向当前 WebView evaluateJavascript 派发 CustomEvent；同 origin 导航也不能把旧响应交给新文档。旧文档失效即取消等待并丢弃回复，媒体按既有 owner 清理。未知版本拒绝，不降级；错误 message 不携带凭据、路径或内部诊断。

| method | params | result / 语义 |
|---|---|---|
| `getStatus` | `{}` | 最小状态摘要，见下文 |
| `refresh` | `{}` | 原生配置同步完成后返回同一状态摘要；仅更新配置，不是聊天/备份/User Sync 或 Feed 刷新，不重放失败运行 |
| `close` | `{}` | `{}`；关闭工作台，不退出企业登录 |
| `logout` | `{}` | 原生展示退出确认，调用同一退出命令；完成后销毁文档，不能等待 JS 回调才清理本地授权 |
| `openExternal` | `{url}` | `{}`；仅宿主批准的 HTTPS 外链，无 userinfo，不在 WebView 内导航 |
| `capturePhoto` | `{}` | 由原生权限与可取消拍摄 UI 获取照片，返回 MediaHandle |
| `recordAudio` | `{maxDurationSeconds}` | 整数 1..60；原生权限与可取消录音 UI，用户明确开始/停止，返回 MediaHandle |
| `readMedia` | `{mediaId,offset,maxBytes}` | offset 为非负安全整数，maxBytes 为 1..65536；返回 `{dataBase64,nextOffset,eof}` |
| `releaseMedia` | `{mediaId}` | `{}`；释放当前文档拥有的媒体与句柄，重复释放本域已释放句柄无副作用 |
| `cancel` | `{targetRequestId}` | `{}`；取消当前文档在途请求，未知/已结束请求无副作用，不取消其他文档调用 |

状态摘要为 `managedReady,appliedManagedGeneration,lastConfigurationSync,lastEnterpriseUpdateRefresh`（未知值为 null）及 `capabilities`（当前宿主实现的方法名数组）。能力声明不是授权：每次执行仍复核身份、来源、系统权限及用户操作。未实现相机/麦克风必须明确不可用，不假报成功；不得因此删除其产品交付要求。

配置同步共用原生企业状态页的同一命令和来源：平台执行 Managed State/Snapshot 校验与原子应用，本地来源执行同等候选验证和提交。无版本变化也算完成一次有效检查，可更新 lastConfigurationSync；成功响应只包含已提交/确认状态，不能用目标 generation 或仅入队状态假报完成。失败保留最后完整配置，不覆盖成功时间，并返回 typed error；已知撤销或运行屏障仍生效。来源/Session 在同步期间变更时取消调用，不将结果发给新文档。

原生按当前来源/主体合并并发同步；每个 bridge requestId 单独关联响应和取消。`cancel` 取消该请求的等待/回调，在提交前且无其他使用者时可取消同步工作；原子提交完成后不回滚配置，不重放运行。网页超时后可由用户重新读取状态或再次同步，不把未知结果当作已失败提交。按钮在能力未声明、会话不可用或本页同步进行中时不可用，并显示对应原因。Feed 的刷新入口独立，不修改配置版本或续期母 Session。

`recordAudio` 是一次包含原生开始/停止 UI 的操作；网页不另行发送未定义的 startRecording/stopRecording。网页“取消”调用 `cancel(targetRequestId)` 丢弃本次录音，成功停止则返回一个 MediaHandle。当前唯一网页传输为 readMedia 的有限 base64 分块，不能自行替换成全量 base64、content URI 或任意资源 URL。把预览进一步保存到聊天/Artifact 需要单独产品动作和合同，不能将本协议的临时句柄当作持久附件。

MediaHandle 为 `{mediaId,mimeType,byteLength}`：mediaId 不可预测且只对当前文档/域/主体/Session 有效；照片固定 `image/jpeg`，录音固定 `audio/mp4`。单项上限 10 MiB，同文档最多两项、总量不超过 20 MiB，有效期最多五分钟；超限返回 `resource_limit`。它不暴露本机路径、content URI、全局 Artifact ID 或任意文件系统。readMedia 只能读取该句柄的范围，nextOffset 必须等于 offset 加实际字节数，eof 仅在 byteLength 末尾为 true。

媒体临时结果归发起时域/主体，默认不写聊天、备份或配置，也不自动上传企业。网页只作当前页面预览，离开时释放 Blob URL 和内存；原生负责在释放、到期、页面替换、切域、退出、母 Session 失效或进程恢复清理未交付临时文件。离开后的迟到结果必须丢弃并清理，不能交付给新页面或重新登录的主体。

普通调用最多 10 秒，拍照/录音最多 120 秒；网页超时发送 cancel，不自动重试原操作。原生自行保证同样的取消/期限，即使 JS 崩溃也不得持续录制。拒绝码至少区分 `invalid_request`、`unsupported_version`、`unsupported_method`、`permission_denied`、`user_cancelled`、`session_expired`、`media_unavailable`、`resource_limit`、`timeout`，配置同步另区分 `network_unavailable`、`configuration_invalid`、`configuration_not_ready`；来源不受信时不向其他文档发送带企业信息的响应。

宿主切换 Realm/Session 或退出时先取消调用、销毁文档并清除该站点 WebView Cookie/cache/存储，确认清理完成后才发布新的空间。清理失败保留可重试状态，不复活旧文档，不清除无关站点数据。采集取消或文档失效时先撤销交付权限，再停止并等待原写入者，最后清理临时文件，避免迟到写入重建文件。退出确认受原文档期限和请求取消约束；已经接受的退出由应用 owner 收口，不依赖 JS 回调。网络失败不等于退出；网页短期会话过期、母 Session 失效和 Access Token 刷新分别按本节 Session 合同处理。JS 侧检查不能替代原生来源和生命周期校验。

具体内存/数据库/密钥存储机制属于 core/Android 实现；跨组件可观察的恢复与认证语义不能下放为单仓库自由实现选择。

## 9. Managed State preflight

BOUND Client 每个新顶层 Managed interaction 前：

```http
GET /api/client/v1/managed/state
Authorization: Bearer <access-token>
X-Measix-Applied-Managed-Generation: N   # 仅确有 Applied Snapshot 时
```

Response：

```text
runtimeStatus READY|ACTIVATING|DEGRADED
activeManagedGeneration >= 0
managedStateRevision
syncRequired
targetManagedGeneration?
runtimeBlocked
```

语义：

- READY + same generation → allow；
- READY + mismatch → sync；
- ACTIVATING/DEGRADED → block；
- revoked → reject；
- Hub unavailable → new Managed runtime fail closed。

不得使用 TTL cache 绕过 correctness preflight。

## 10. Managed Draft / Snapshot contract

### 10.1 ManagedDraftContent

当前 Draft 的资源内容：

```text
providers: ProviderDefinition[]
models: ModelDefinition[]
tts: TtsDefinition[]
asr: AsrDefinition[]
mcp: McpDefinition[]
bindings: RuntimeBindingDefinition[]
policy: ManagedPolicy
```

当前 Draft 同时包含以下 Experience 内容，并采用 §10.10.1 五项策略：

```text
assistants: ManagedAssistantDefinition[]
starters: AssistantStarterDefinition[]
```

S0.3 的当前 v5 设计在 v4 上增加 Gateway control-plane content（原 v3 为保留草案，不是当前发布版本）：

```text
toolGatewayProfile?: ToolGatewayProfile
toolIntegrations: ToolIntegration[]              # server-only
candidateGatewayCatalog                          # server-only review workspace
publishedGatewayCatalog input                    # compiler input, server-only
```

`ToolGatewayDefinition` 是上述 Profile/Published Catalog 编译出的只读 client projection（§10.8），不是 Draft 内第二份可独立编辑的 authority。Catalog review workspace 与 immutable published input 也不可混为同一可变对象。

Enterprise Update 使用独立 Draft/Publish/Withdraw 与 Feed revision，不进入 `ManagedDraftContent`。

### 10.2 S0.1 client protocol vocabulary

Protocol vocabulary can be extensible, but **S0.1/S0.4 required and VERIFIED runtime baseline** is exactly：

```text
OPENAI_CHAT_COMPLETIONS
OPENAI_AUDIO_SPEECH
OPENAI_AUDIO_TRANSCRIPTIONS
MCP_STREAMABLE_HTTP
```

Compatibility-extension values such as：

```text
OPENAI_RESPONSES
ANTHROPIC_MESSAGES
```

may remain reserved/implemented later, but their presence in an executable enum does **not** imply S0.1 product support. Admin must not present unqualified values as normal supported choices.

### 10.3 ProviderDefinition

```text
providerId       prv_*
displayName
clientProtocol
enabled
```

S0.1 ProviderDefinition is client-side grouping/protocol metadata, not Upstream infrastructure. No base URL/API key/SecretRef.

### 10.4 ModelDefinition

```text
modelId          mdl_*
providerId       prv_*
displayName
upstreamModelKey
runtimePath
inputModalities[]
outputModalities[]
capabilities[]
enabled
```

S0.1 vocabulary：

```text
inputModalities  TEXT | IMAGE
outputModalities TEXT
capabilities     TOOL | REASONING
```

Unknown request value is validation error for the frozen S0.1 profile. Future capability vocabulary extension requires explicit client compatibility behavior; old clients must not silently assume unsupported capability semantics.

### 10.5 TtsDefinition

```text
ttsId             tts_*
displayName
clientProtocol     OPENAI_AUDIO_SPEECH
upstreamModelKey
voice              required non-empty
runtimePath
enabled
```

`voice` is part of the Managed execution definition and must not be supplied by an implicit Android default.

S0.1 required response audio profile is MP3; it is a release profile rule, not a generic codec DSL field.

### 10.6 AsrDefinition

```text
asrId              asr_*
displayName
clientProtocol      OPENAI_AUDIO_TRANSCRIPTIONS
upstreamModelKey
runtimePath
language?           optional default language
enabled
```

S0.1/S0.4 ASR semantics are HTTP multipart transcription. Realtime/WebSocket/VAD/sample-rate provider configuration is not part of this Managed definition.

### 10.7 McpDefinition

```text
mcpServerId         mcp_*
displayName
clientProtocol      MCP_STREAMABLE_HTTP
runtimePath
authOwnership       ENTERPRISE_MANAGED | NONE
enabled
```

`USER_MANAGED` Managed MCP is not defined in S0 because per-user delegated upstream credential transport is intentionally deferred. Android Local MCP OAuth remains Local capability.

`enabled=true` 的 Direct Managed MCP 自动进入所属 Enterprise Realm 的可用资源目录，不要求用户复制或逐个开启；模型实际 tools 仍按 Assistant 的 `mcpServerIds[]` 选择，不变成全局内置工具。Personal Realm 不得注册、解析或执行它。

### 10.8 ToolGatewayDefinition

```text
toolGatewayId      twg_*
displayName
clientProtocol     MCP_STREAMABLE_HTTP
runtimePath
surfaceVersion     1
surfaceHash        sha256:<hex>
clientEnablementPolicy REQUIRED | USER_CONTROLLABLE_DEFAULT_ON
```

Snapshot/client 只消费该 logical resource、expected surfaceHash 与 client enablement policy；完整 `discover_tools` / `invoke_tool` definitions 由 Gateway 的标准 MCP `tools/list` 返回。客户端必须校验 canonical `tools/list` surface hash；不允许从 Snapshot 自行拼接平行 tools。Canonical hash 输入是固定顺序的两个完整 MCP Tool object 的 JSON array，不含 JSON-RPC id/envelope、pagination cursor 或 transport metadata。

`clientEnablementPolicy` 必填，缺失/未知值使候选 Snapshot 验证失败。S0.3 `surfaceHash` 为上述数组按 [RFC 8785 JCS](https://www.rfc-editor.org/rfc/rfc8785) 得到的 UTF-8 canonical bytes 的 SHA-256，编码为 `sha256:<lowercase hex>`；不允许随请求变化的 Tool object metadata。此规则仅定义新的 Gateway surface 摘要，不建立旧 Snapshot 格式的兼容义务。

`ToolGatewayDefinition` 的存在表示该 Release 已发布 Gateway；Draft/Profile 的 `enabled=false` 由 compiler 表达为 Snapshot 中省略该 resource，不再在 wire 中保留第二个 enabled bit。两个政策值语义：

- `REQUIRED`：Enterprise Realm 必须开启，客户端关闭写入被拒绝；
- `USER_CONTROLLABLE_DEFAULT_ON`：无本地 preference 时开启，允许用户成对关闭/开启。

两个 Meta Tool 是不可拆分的原子 built-in pair，绝不逐工具 toggle。该 preference 是 Android Enterprise-local state，不回写 Snapshot，也不改变 `managedGeneration`，只影响下一 interaction 捕获的 effective tool set；立即停止在途工作使用显式 cancellation。

### 10.9 RuntimeBindingDefinition — server-only

本节是 Direct Model/TTS/ASR/MCP 的 Upstream binding。`twg_*` 的 Gateway route 由 Hub 根据已发布 Gateway profile 与私有服务配置编译，遵守 §6.1 的 targetKind，不伪造 `upstreamId` 或要求 Admin 手写 Gateway route。

```text
runtimeRouteId       rte_*
resourceId
upstreamId           ups_*
allowedMethods[]
allowedPathPrefixes[]
transportPolicy
timeoutPolicy?
```

Binding is Draft/server runtime input and **never appears in Client Snapshot**.

### 10.10 ManagedPolicy

```text
policyId             pol_*
allowLocalProviders
allowLocalTts
allowLocalAsr
allowLocalMcp
allowLocalAssistants  # required
defaultModelId?
defaultTtsId?
defaultAsrId?
```

No User/Group assignment in S0.

当前五项 `allowLocal*` 的类型对应与准入语义见生命周期架构 §4.1：允许时直接引用用户唯一配置及其用户凭据，禁止时仅影响企业域可用性，运行数据仍按域和主体隔离。清单外准入及主/子助手引用按该节独立规则执行，助手开关不连带授权其他受控资源。

### 10.10.1 未发布平台的唯一当前协议

MEASIX 后端尚未发布，当前 Snapshot schemaVersion=4 是唯一有效的 S0.2 数据格式。旧 v1/v2/v3 编号、测试快照与“已冻结”原型不构成兼容承诺；不提供旧策略采用、旧 Release 升级、双读或迁移旁路。服务端只声明当前支持版本，未知或旧 Snapshot 版本明确拒绝。

新建 Policy 的五项开关默认 false；请求和持久化当前文档显式包含全部五项 Boolean，缺失/null/错误类型均为无效内容，不补默认值掩盖坏输入。默认值只属于新建动作，不属于解码兼容。

当前 canonical policy 包含 allowLocalAssistants，资源/Experience 数组及 hash 遵循 §10.13–14。既有编号保留用于准确识别当前协议，不代表存在四个可运行版本。后续 S0.3 Gateway 的 v5 是尚未实施的目标，不进入当前 Discovery 或 Snapshot 消费。

数据库亦以当前完整 schema 为唯一初始化目标；不维护未发布原型的增量迁移、字段回填或版本转换。初始化、当前版本校验、备份与崩溃恢复仍须真实执行，不能把兼容清理变成跳过完整性检查。

### 10.10.2 S0.2 接入版本与应用边界

真实 S0.2 接入的目标组合为 Discovery protocolVersion `1`、Snapshot `4`、Portal Bridge `3`；本地读取 `localReadVersion=2` 只适用于本地宿主。接入资料 formatVersion=1 独立于这些版本。Discovery/Bootstrap 必须声明实际可提供的版本；只声明当前实现版本，旧原型资料不作为交付输入。Gateway 不属于本次 v4 profile。

可执行合同必须要求 schemaVersion=4、assistants/starters 集合和五项非 null Boolean，不保留 optional 兼容读类型。HTTP 响应扩展仍遵循 §2.1，已知字段的类型、枚举、版本及同快照引用必须验证；Bridge 的封闭信封规则不推广到所有 HTTP 响应。客户端不得因忽略未知 optional 字段而接受未知 schema 或上游凭据字段。

认证成功但尚无 active Release 时只进入已接入/待配置；不得构造 generation 0 Snapshot。Bootstrap/Managed State 决定目标 generation，Snapshot 通过当前授权、deployment、schema、引用和 ETag/body hash 校验后整体替换 Applied State。用户配置与本域偏好不回写 Snapshot。读取过程中撤销授权不得因已有 ETag 绕过校验。

Refresh 采用 §8 的同 token/同 Idempotency-Key 恢复规则；未确认响应的客户端保留原轮换事务，不把网络失败当作退出或新轮换成功。普通请求不续 idle。已明确 forwarded=false 的 428 只触发配置恢复并等待下一次用户动作；超时或其他未知下游结果不自动重放。退出先完成本地撤权与资源收口，远端撤销未确认单独呈现。

### 10.11 Runtime path

`runtimePath` must be path-only, normalized, start `/`, contain no scheme/host/query/fragment and fall inside its RuntimeBinding allowlist. Request query string may be passed through according to Relay route policy.

### 10.12 当前 Snapshot 完整结构

```http
GET /api/client/v1/managed/snapshots/{generation}
If-None-Match: "<snapshotHash>"
```

Body：

```text
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
metadata { publishedAt, publishedByUserId? }
```

Snapshot never contains：

```text
bindings
upstreamId
Upstream base/internal URL
runtimeRouteId
SecretRef resolved value
resolved credential
PricingRule
```

Only ACTIVE/SUPERSEDED finalized Release snapshot can be downloaded; STAGED/ACTIVATION_FAILED is not client-visible.

### 10.13 Snapshot hash / ETag

`snapshotHash = sha256:<hex>` from deterministic descriptor excluding `snapshotHash` itself; entity arrays sorted by stable ID.

Client does not invent another JSON canonicalization. It verifies HTTP ETag == body snapshotHash plus TLS/schema/reference/deployment identity, then atomically commits.

### 10.14 当前 Snapshot 的 Enterprise Experience 字段

当前 Snapshot 包含下列 Experience 字段：

```text
schemaVersion = 4
assistants[]
  assistantDefinitionId asd_*
  displayName
  description?
  systemPrompt
  modelId mdl_*
  memorySeed[]
  mcpServerIds[]
  enabled
starters[]
  starterId str_*
  assistantDefinitionId asd_*
  title
  description?
  prompt
  sortOrder
  enabled
```

`memorySeed[]` 是 generation-bound、只读的成熟企业经验；不映射为 Android 可变 Local Assistant Memory。它可为空数组；存在的每项 trim 后必须非空，保留作者顺序，不按字典排序或去重。Assistant/Starter reference 必须在同一 Snapshot 内闭合。Canonical Snapshot 的 assistants/starters 集合分别按自身 stable ID 升序，mcpServerIds 按 ID 排序；客户端显示同一 Assistant 的 Starter 时按 `(sortOrder, starterId)` 排序，不用展示顺序改变 canonical hash。Skill 不进入当前 v4。

这些规则必须由当前完整样例及 compiler 的 golden 校验共同验证；删除未发布旧版本样例，不复用旧原型 Freeze 声明。

### 10.15 后续 Gateway Snapshot 目标（当前不提供）

后续 Gateway 目标在当前 S0.2 字段上增加：

```text
schemaVersion = 5
toolGateway?: ToolGatewayDefinition   # complete definition in §10.8; no enabled bit
```

Snapshot v3 不包含完整 Meta Tool definitions、Tool Catalog、Tool Integration、downstream URL、Secret、toolRef、Gateway internal URL/control/index。`surfaceHash` 由与 Admin Preview/GatewayControlState 完全相同的 canonical surface compiler 计算。

### 10.16 Enterprise Update Feed 与 Gateway platform tool

企业动态使用独立 Feed authority/revision/ETag，不进入任何版本 Managed Snapshot，也不改变 `managedGeneration`。Client/Portal 读取接口与 S0.3 Gateway platform tool 使用相同过滤语义：

```http
GET /api/client/v1/enterprise/updates?startDate=&endDate=&limit=
GET /api/client/v1/enterprise/updates/{enterpriseUpdateId}
If-None-Match: "<feed-etag>"
```

Admin 公开 API 必须提供 Enterprise Update 的 Draft create/update、Publish 和 Withdraw 命令；具体 request shape 进入 S0.2 executable Admin OpenAPI。Publish/Withdraw 改变 Feed revision/ETag，不改变 Managed Draft/Release/generation。

```text
startDate?               YYYY-MM-DD, inclusive (Client/Portal/private HTTP)
endDate?                 YYYY-MM-DD, inclusive (Client/Portal/private HTTP)
limit?                    default 10, range 1..20
```

- 两个日期都省略：按 `publishedAt` 降序返回最新 `limit` 条；
- 仅 `startDate`：从该日到企业当前日期；仅 `endDate`：截至该日；两者都有：闭区间；
- `startDate > endDate` 返回 `invalid_request`/typed invalid-argument；
- 日期按 Deployment timezone 解释；超过 limit 返回 `truncated=true`；
- 输出只包含 `enterpriseUpdateId`、title、content、contentFormat、category、severity、publishedAt 和 enterprise timezone，不包含 Draft/Withdrawn 内容。

Published Gateway Tool 的 agent-facing name 固定为 `get_enterprise_updates`。Portal/Client Feed projection 与 Gateway tool projection 可不同，但内容真源和过滤语义必须相同。

仅该 platform tool 的业务参数/结果投影保留 Realm/Experience Contract §5.2 的 `start_date/end_date/enterprise_timezone/update_id` 等 snake_case 名称；它们是工具 schema 的字段，不是 Client/Portal HTTP aliases。`discover_tools` 外层仍固定 `limitPerQuery`。Client Feed 使用 `enterpriseTimezone`、`items[].enterpriseUpdateId`、`contentFormat`、`publishedAt`、`truncated`。

Feed revision 必须在使可见内容变化的 Publish/Withdraw 事务中单调推进；仅编辑不可见 Draft 不推进公开 Feed revision。一次列表读取的 items/truncated/ETag 必须来自同一一致视图；ETag 标识规范化查询及其表示，不能只依赖不含查询上下文的全局 revision。按企业日期解释的 start-only 查询跨午夜时也必须重新判断，不能错误返回 304。日期日界线按 Deployment timezone 的日历日期计算，不把夏令时日期固定视为 24 小时。

S0.3 由 Enterprise Tool Gateway 的 platform adapter 调用 Hub private typed read API；Hub 不提供 MCP initialize/tools/list/tools/call projection。Gateway/Android/Portal 不得访问 Admin mutation surface，private API 不进入 public ingress。

## 11. S0.3 Gateway MCP surface

Gateway 对 authorized generation 的标准 MCP `tools/list` 必须返回且只返回：

```text
1. discover_tools
2. invoke_tool
```

两工具的 name/order/description/inputSchema/outputSchema 在同一 generation 内 canonical/byte-stable。Gateway 不通过 `tools/list_changed` 把 Candidate drift 热推给当前 interaction；新 surface 只随新 Managed generation 被新 interaction 消费。

`discover_tools` inputSchema 语义：

```text
queries           Array<String>, required, minItems=1, maxItems=5
limitPerQuery?    Integer, default=3, minimum=1, maximum=5
additionalProperties=false
```

其 structured result/outputSchema 语义：

```text
catalogGeneration Integer       # exactly captured managedGeneration
results[]
  query String
  matches[]
    toolRef String
    gatewayToolId gtl_*
    name String
    description String
    inputSchema JSON Schema Object
    outputSchema? JSON Schema Object
    risk READ_ONLY
    expiresAt RFC3339
```

自然语言、精确 published agent-facing name 和 alias 都使用同一入口；match 必须带完整 published input schema，存在时带 output schema。不另增 inspect/schema tool。

`invoke_tool` inputSchema 语义：

```text
toolRef       opaque String, required
arguments     Object, required
additionalProperties=false
```

Gateway 只接受 discover 签发并绑定 `deploymentId/userId/deviceId/sessionId/interactionId/managedGeneration/gatewayToolId/schemaHash/expiry` 的 ref。签名/完整性、scope、expiry、authorization、catalog/schema hash 和 arguments validation 全部通过后才可 downstream forward；不接受 tool name/integration/endpoint/schema fallback。

Tool-level failure 使用 MCP tool result `isError=true` + stable machine code，至少区分：

```text
tool_ref_invalid
tool_ref_expired
tool_ref_scope_mismatch
gateway_tool_not_found
tool_arguments_invalid
gateway_catalog_drift
downstream_tool_error
downstream_outcome_unknown
```

可安全重试性只能来自该 error contract；forward 后 timeout/network uncertainty 不能伪造成明确“未执行”。Resolved business action metadata 放在 client-safe namespaced result metadata `com.measix/resolvedTool`，至少含 `gatewayToolId/name/status/requestId`；模型/客户端都不得从任意 downstream 文本反解析身份。Metadata 不包含 source endpoint、credential、toolRef claims 或 raw arguments。

## 12. Admin Snapshot Preview

Admin must expose a read-only preview derived from the **same canonical Snapshot projection** used by Release compilation：当前 S0.2 preview 使用 v4，包含 Assistant/Memory Seed/Starter；S0.3 目标 preview 使用 v5 并额外展示 ToolGatewayDefinition、最终两工具 surface 和 surfaceHash。

Preview：

- does not create Release/generation；
- does not change runtime；
- must show client-visible providers/models/tts/asr/mcp/policy，当前 v4 显示 assistants/starters，未来 v5 显示 Gateway logical resource + surfaceHash；
- must not contain binding/upstream/internal route/Secret/Pricing；
- exact Admin endpoint is executable-contract detail, but must be represented in Admin OpenAPI before implementation。

## 13. Runtime Request Contract

URL：

```text
/runtime/v1/resources/{resourceId}{runtimePath}
```

Headers：

```http
Authorization: Bearer <access-token>
X-Measix-Managed-Generation: <interaction generation>
X-Measix-Interaction-Id: int_<uuid>
```

`resourceId` path is the sole logical resource identity (`mdl_*/tts_*/asr_*/mcp_*/twg_*`); no `X-Measix-Resource-Id`.

Relay creates：

```http
X-Measix-Request-Id: req_<uuid>
```

and may forward that correlation header to qualified Adapter.

Admission order before body forward：

```text
parse resource/path
→ authenticate
→ resolve Principal/security state
→ generation barrier
→ resource exists in active resourceRoutes
→ route resolve
→ method/path policy
→ sanitize/inject upstream or Relay→Gateway service credential
→ for Gateway target: inject integrity-protected Principal envelope
→ start upstream transport
```

## 14. Required Runtime transport semantics

| Capability | S0.1/S0.4 required transport |
|---|---|
| Model | HTTP request/response + streaming/SSE passthrough |
| TTS | request + binary MP3 response |
| ASR | multipart upload + JSON transcription response |
| MCP | MCP Streamable HTTP |

Gateway resource 仍是 MCP Streamable HTTP；Relay 透明转发 JSON-RPC，不解析 `discover_tools` query、`toolRef`、真实工具名或 arguments。Gateway business error/result 不改写为 Relay business semantic。

Relay does not parse Provider body, translate model schemas, rewrite audio fields or infer semantic usage.

WebSocket is not supported in S0 runtime contract.

## 15. 428 generation contract

Stale generation：

```http
428 Precondition Required
```

Problem：

```text
code = managed_snapshot_required
targetManagedGeneration
requestId
forwarded = false
```

Strong invariant：Adapter/Upstream/Gateway has received no request body.

Client：terminate current interaction → sync → create new interaction if user/upstream retries. Never replay existing user/tool/MCP side effect in the same interaction.

## 16. Upstream / Secret Admin contract

### Upstream endpoints

```text
GET/POST /api/admin/v1/upstreams
GET/PUT  /api/admin/v1/upstreams/{upstreamId}
POST     /api/admin/v1/upstreams/{upstreamId}:test
POST     /api/admin/v1/upstreams/{upstreamId}:apply
POST     /api/admin/v1/secrets
POST     /api/admin/v1/secrets/{secretId}:replace
```

`UpstreamConfig`：

```text
name
baseUrl
transportCapabilities[]
auth
  NONE
  | BEARER { secretRef }
  | STATIC_HEADER { headerName, secretRef }
  | BASIC { username, passwordSecretRef }
correlationMode
usageCapabilityLevel LEVEL_0|LEVEL_1|LEVEL_2
timeoutDefaults { connectMs, responseHeaderMs, idleMs }
```

`secretRef = {secretId,secretVersion}` references immutable SecretVersion.

Save candidate only increments `configRevision`; `:apply` Relay ACK then changes `activeConfigRevision`.

Secret replace creates new version only; it does not mutate candidate or active Upstream automatically.

`baseUrl` cannot contain userinfo/query/fragment. Runtime Client never supplies scheme/host/port.

`:test` tests candidate only and returns reachability/latency/verified capabilities/warnings without changing active runtime.

## 17. Admin Draft / Release / Activation contract

Draft：

```text
GET  /api/admin/v1/draft
PUT  /api/admin/v1/draft
POST /api/admin/v1/draft:validate
POST /api/admin/v1/draft:publish
```

PUT is full replacement + `expectedDraftRevision`; no JSON Patch. Revision mismatch → `409 stale_draft_revision` + `currentDraftRevision`.

Validation response：

```text
valid
errors[]
warnings[]
```

`ValidationIssue`：

```text
code
severity ERROR|WARNING
path
message
```

Publish includes `expectedDraftRevision + acknowledgedWarningCodes[]` and Idempotency-Key.

Release/Activation：

```text
GET  /api/admin/v1/releases
GET  /api/admin/v1/releases/{releaseId}
POST /api/admin/v1/releases/{releaseId}:republish
GET  /api/admin/v1/activations/{activationId}
```

Activation fields at least：

```text
activationId
kind PUBLISH|RUNTIME_CONFIG|SECURITY_CHANGE
state APPLYING|COMPLETED|FAILED|UNKNOWN
releaseId?
subjectId?
targetManagedGeneration?
desiredControlRevision
bundleHash?
createdAt/updatedAt
errorCode?
```

Timeout alone is not `FAILED`; reconciliation resolves uncertainty.

## 18. Request Usage ingest

Relay creates RequestUsageEvent only after an Enterprise Principal is successfully parsed.

```text
requestId
interactionId?
deploymentId
userId
deviceId?
resourceId?           # validated resource identity when available
runtimeRouteId?       # only when a route was resolved
targetKind?           # UPSTREAM | ENTERPRISE_TOOL_GATEWAY, only when resolved
upstreamId?           # only for a resolved UPSTREAM target
managedGeneration
requestedManagedGeneration? # parsed request header, distinct from applied state
controlRevision
startedAt
completedAt
forwarded
httpStatus
upstreamHttpStatus?
requestBytes
responseBytes
durationMs
errorClass?
```

已认证但 resource/route 未命中的拒绝请求仍应产生 `forwarded=false` 事实；不为补齐字段伪造 route/upstream，不保存未经验证的原始 resource 字符串。已转发请求必须有 resource/route/targetKind；Gateway 请求的 resource 为 `twg_*`，没有虚假 `ups_*`。`managedGeneration/controlRevision` 表示捕获的 Relay applied state；如需记录有效解析的请求代次，应使用独立 `requestedManagedGeneration`，不得把两者混写。没有可信 Principal 的拒绝只进入安全诊断，不伪造用户 Usage。Gateway tool execution fact 另外关联 requestId，不与 Relay request fact 重复计数。

Relay appends to its durable local spool, then batches：

```http
POST /internal/v1/usage/request-events:batch
```

Hub dedupes by requestId. Batch 200 means every row accepted or already exists; only then Relay deletes rows. Invalid row/batch must be isolated/diagnosed and not silently discarded or block all good rows forever.

Semantic Usage is separate from transparent RequestUsage and comes from qualified Adapter/Hub connector where available.

## 19. Semantic Usage / Pricing vocabulary

Standard meters for S0.1 required profile：

```text
INPUT_TOKENS
OUTPUT_TOKENS
CACHED_TOKENS
CHARACTERS
AUDIO_SECONDS
REQUESTS
```

Resource meaning：

```text
MODEL  → token meters + REQUESTS
TTS    → CHARACTERS / AUDIO_SECONDS / REQUESTS
ASR    → AUDIO_SECONDS / REQUESTS
MCP    → REQUESTS
```

Completeness：

```text
EXACT | PARTIAL | UNKNOWN
```

Cost status：

```text
KNOWN | PARTIAL | UNKNOWN
```

No reliable semantic meter → no token/audio guessing from byte counts.

PricingRule：

```text
pricingRuleId
resourceId?
upstreamId?
meter
unitSize
unitPrice
currency
effectiveFrom
```

Pricing uses whole ruleset + optimistic revision. No implicit FX conversion.

## 20. Admin Usage query contract

S0.1 product requires request listing/filter semantics for：

```text
Time range
User
Resource
Resource Kind MODEL|TTS|ASR|MCP (S0.1); ENTERPRISE_TOOL_GATEWAY (S0.3)
Upstream
Status
Usage completeness
```

Executable Admin OpenAPI must expose stable query parameters before implementation; exact parameter names are owned by that executable contract but must preserve these semantics.

`UsageSummary` must be able to represent：

```text
request/forwarded/error/blocked counts
request/response bytes
semantic meters + completeness
resource-kind breakdown
cost status/amount/currency when meaningful
```

`RequestUsageView`/detail includes request correlation and semantic/cost details but never Secret/header/prompt/body.

列表与汇总必须应用同一套显式筛选，不得只过滤 request counts 而把其他用户/资源的 semantic meters 或 cost 汇入结果。Request completeness 由已关联的 Semantic Usage 聚合：无记录或任一 UNKNOWN 为 UNKNOWN，否则任一 PARTIAL 为 PARTIAL，否则为 EXACT。未关联 requestId 的 provider-level Semantic Usage 可进入未按用户/请求状态限定的 upstream/resource 汇总；不得推断其 User 或混入用户/请求状态筛选结果。语义记录的 resource/upstream 过滤使用已验证归属，时间窗为下界包含、上界排除。部分缺少成本时不能将可得小计标为完整 KNOWN。

## 21. System status contract

Admin `/system/status` at least exposes：

```text
buildVersion
dbHealth
schemaIdentity
runtimeStatus
activeManagedGeneration
managedStateRevision
desiredControlRevision
desiredBundleHash
relayReady
appliedControlRevision
appliedBundleHash
lastRelaySeenAt
desiredGatewayControlRevision?
desiredGatewayBundleHash?
gatewayReady?
appliedGatewayControlRevision?
appliedGatewayBundleHash?
lastGatewaySeenAt?
latestActivation?
requestUsageIngestLagSeconds?
semanticOrphanCount
```

S0.1 may add optional response fields such as spool/backlog/unknown summaries when needed by the executable Admin UI; current consumers ignore unknown optional fields, and Secret/credential cannot appear.

## 22. Auth boundary

- Admin API：Admin Session cookie + CSRF；
- Client API：Access JWT aud includes `client`；
- Runtime：Access JWT aud includes `runtime`；
- Hub↔Relay、Hub↔Gateway、Relay↔Gateway：separate scoped service credentials；
- public/admin/client same-origin by default, no wildcard CORS；
- internal API no public CORS/public ingress。

## 23. Problem contract

Common shape：

```text
type
title
status
code
detail?
requestId?
activationId?
targetManagedGeneration?
currentDraftRevision?
forwarded?
```

Stable codes at least：

| HTTP | code |
|---|---|
| 400 | `invalid_request`, `invalid_id`, `unsupported_client_protocol` |
| 401 | `unauthenticated`, `session_expired` |
| 403 | `forbidden`, `user_disabled`, `device_revoked`, `session_revoked`, `resource_not_allowed` |
| 404 | `resource_not_found`, `snapshot_not_found`, `release_not_found`, `activation_not_found` |
| 409 | `stale_draft_revision`, `stale_upstream_config_revision`, `stale_secret_version`, `username_conflict`, `idempotency_conflict`, `stale_control_revision`, `control_revision_hash_conflict`, `stale_gateway_control_revision`, `gateway_control_revision_hash_conflict` |
| 422 | `validation_failed`, `invalid_runtime_control`, `invalid_gateway_control`, `invalid_usage_batch` |
| 428 | `managed_snapshot_required` |
| 502 | `upstream_protocol_error` |
| 503 | `runtime_control_unavailable`, `gateway_control_unavailable`, `runtime_control_activating`, `upstream_unavailable`, `enterprise_configuration_unavailable` |
| 504 | `upstream_timeout` |

Relay-generated generation deny must return `forwarded=false`.

Upstream 4xx/5xx after forwarding is normally protocol-transparent; Relay transport failures use `upstream_*`. Redirect with Location is rejected as `upstream_protocol_error`; upstream Set-Cookie is not exposed to client.

## 24. S0 failure semantics

| Scenario | Behavior |
|---|---|
| Hub unavailable | new Managed interaction blocked; existing Relay streams may finish under captured state |
| Relay unavailable | Runtime fails; Admin/Client control plane remains readable if Hub healthy |
| Relay restart/no control | runtime 503 fail-closed until Hub rehydrates |
| Gateway unavailable | Direct resources may remain usable; Gateway resource fails without Relay business fallback |
| Gateway restart/no control | Gateway runtime 503 fail-closed until Hub rehydrates exact Gateway control |
| Adapter unavailable | upstream error; no Snapshot/control mutation |
| Snapshot download invalid/fail | keep LKG but not READY for new Managed runtime |
| Usage ingest outage | Relay durable spool/replay |
| Publish timeout | Hub reconciles all affected Gateway/Relay revision/hash; no duplicate generation |

## 25. Contract / Stage gates

S0.1 Contract Gate must prove：

1. required profile typed semantics including TTS voice / HTTP ASR / MCP auth ownership；
2. Snapshot no Secret/upstream/runtimeRoute；
3. Snapshot preview same canonical projection；
4. four required transports work via real Hub/Relay + deterministic Adapter/Test Client；
5. old generation/invalid auth/resource deny before body forward；
6. Upstream candidate/active/Secret Apply semantics；
7. Usage durable + resource-kind/filter/pricing semantics；
8. OpenAPI/fixtures/generated types no drift；
9. 阶段候选 manifest 记录当前 schema v4 及实际证据基线。

S0.2 must prove Snapshot v4、五项用户配置准入及严格校验、Personal/Enterprise 运行数据隔离、Managed Assistant/Memory Seed/Assistant Starter、Enterprise Update Feed、七天滚动 Session 与 Portal/手机能力；不再以 Hub MCP projection 作为 Exit。

S0.3 must consume the frozen S0.2 baseline and prove Snapshot v5、Gateway standard two-tool surface/surfaceHash、Candidate/Published lifecycle、discover/toolRef/invoke、Gateway-first/Relay-second activation、platform + external MCP、restart/security/search-quality semantics with Admin + Test Client, while retaining S0.1/S0.2 regression evidence.

S0.4 must consume the pinned S0.3 baseline and prove Android Enrollment/Keystore/LKG/Effective Runtime/Model/TTS/HTTP-ASR/Direct MCP/Gateway/428/UI semantics while retaining S0.1–S0.3 regression evidence.

Final S0 Exit remains governed by `measix-s0-system-testing-spec.md` and requires real Android emulator/device + Admin browser + real Hub/Gateway/Relay + qualified Adapter/downstream MCP evidence.
