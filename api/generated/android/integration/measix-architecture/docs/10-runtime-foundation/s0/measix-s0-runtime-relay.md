# S0 Runtime Relay 架构与需求规格

> 状态：S0 Component Architecture / 组件架构基线  
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`  
> 术语/ID 权威：`../../00-platform/measix-platform-terminology-and-identifier-contract.md`  
> 协议权威：`measix-s0-control-protocol.md`  
> Upstream 契约：`measix-s0-upstream-adapter-contract.md`  
> 实施决议：`measix-s0-implementation-decision.md`  
> 测试：`measix-s0-runtime-relay-testing-spec.md`  
> 文档职责：定义 Runtime Relay 的数据面职责、control-state ownership、admission/transport/security/metering/recovery invariants；具体 package/file、proxy primitive、spool DDL、运行配置名和当前实现由 `measix-platform-core` 维护。

## 1. 组件定位

Runtime Relay 是企业 Runtime 的 **identity-aware, policy-aware L7 relay**。它位于 Managed Client 与 Upstream Adapter 之间，把 Control Hub 已确定的身份、generation、resource、route、credential 和运行约束落实到每一次 Runtime request。

Relay 回答：

1. 请求代表哪个 authoritative principal；
2. 客户端 generation 是否仍是 active generation；
3. `resourceId` 是否属于当前 published runtime set；
4. 该 resource 当前应使用哪个 server-side route/upstream 或 private Gateway target；
5. method/path/header/credential policy 是否允许；
6. 请求事实如何形成 durable Request Usage。

Relay **不是** Provider protocol translator、业务控制状态真源、Hub DB replica 或 Agent Runtime。

## 2. 与其他组件的边界

```text
Android / future Agent Runtime
        │ public runtime request
        ▼
Runtime Relay
        │ transparent provider-compatible HTTP / MCP
        ├──────────────► Upstream Adapter ─► Provider / Direct MCP server
        └──────────────► Enterprise Tool Gateway ─► platform/downstream MCP

Control Hub
   ├─ full desired RuntimeControlState ─────► Relay
   ├─ applied status ◄────────────────────── Relay
   └─ RequestUsage batch ◄───────────────── Relay
```

不变量：

- Runtime data path 不同步查询 Control Hub；
- Relay 不访问 Hub persistence，也不 import Hub domain/service；
- Hub 不代理大流量 Runtime body/stream；
- Relay 只消费编译后的 runtime control，不解释 Draft/Release；
- Provider-specific compatibility 属于 Adapter；
- Catalog/search/toolRef/schema/downstream tool semantics 属于 Enterprise Tool Gateway；
- Client 永远不知道 server credential、upstream internal address 或 runtimeRouteId。

## 3. RuntimeControlState

Relay 的唯一业务运行 authority 是 Hub 下发并成功应用的完整 immutable control state。精确 wire 以 Control Protocol 为准，语义至少包含：

```text
control revision + bundle identity
active managed generation
deployment/auth key material needed for verification
principal restrictive state
resource → internal route mapping
route execution policy
active upstream runtime configuration
private Gateway target/service credential/envelope policy
operational limits
```

S0 没有 per-user/group resource assignment；active principal 能否使用某 resource 由当前 published resource set + principal security state 决定。

### 3.1 Full-state atomic apply

Apply 必须：

```text
decode
→ validate complete references/security/path/auth material
→ build complete candidate runtime view/indexes
→ atomically replace current state
→ ACK exact control revision + bundle hash
```

任何 validation/apply failure 都必须让旧 Current State 完整保持可用；不能逐 map/route 暴露半新半旧状态。

Revision/idempotency matrix 以 Control Protocol 为权威：same revision/same hash 幂等，stale 或 same revision/different hash 必须拒绝。

### 3.2 Request state capture

每个 accepted request 在进入执行链时 capture 一份 current immutable state/context，并在整个 request/stream 生命周期内使用同一份。并发 apply 新 revision 只能影响后续新 request，不能让在途 stream 中途切换 route/credential/generation。

具体 atomic primitive 属于 core implementation。

## 4. 启动、重启与 fail-closed

Relay 不持久化完整 RuntimeControlState/credential 作为独立业务 authority。

```text
process start
→ no valid runtime control
→ public Managed Runtime fail closed
→ internal health/control endpoint available
→ Hub rehydrates current desired state
→ Relay validates + atomic apply
→ READY
```

如果 Hub 短时不可达但 Relay 进程未重启，Relay 可以继续使用已成功应用的 current state。Relay restart 后没有有效 state 时不能用 stale disk copy 偷偷继续运行。

Usage spool 是独立 durable exception，因为它保存已经发生的 request facts，而不是可由 Hub 重建的 desired control。

## 5. Public Runtime identity

Managed Runtime URL 只使用 client-visible resource identity：

```text
/runtime/v1/resources/{resourceId}{runtimePath}
```

Client request 还携带 enterprise runtime auth、managed generation 和 interaction correlation；Relay 生成自己的 request identity。精确 header/problem contract 以 Control Protocol 为准。

客户端不能提交：

```text
upstreamId
runtimeRouteId
arbitrary host/base URL
server credential
platform requestId override
```

## 6. Admission order

请求在向 Upstream 产生业务 side effect 前必须完成：

```text
allocate platform request correlation
→ authenticate token / resolve principal
→ enforce disabled/revoked principal state
→ validate managed generation
→ validate resource identity is active
→ resolve internal route/upstream or Gateway target
→ validate method/path/size policy
→ sanitize client headers
→ inject server-side upstream or Relay→Gateway service credential/correlation
→ for Gateway target inject integrity-protected Principal envelope
→ only then forward body/stream
```

关键要求：**Auth / principal / generation / resource / route/path 任一失败时，Upstream 不得收到业务 body。**

这比最终 HTTP status 更重要，必须由测试 Adapter/peer 证明 no-forward。

## 7. Generation barrier

S0 新的 Managed Runtime request 只接受：

```text
request.managedGeneration == activeManagedGeneration
```

不匹配返回 Control Protocol 定义的 `managed_snapshot_required` / 428 类结果，并在 forward 前终止。

428 表示当前 request 没有被 forward，可以驱动 Client 进入 Managed Snapshot update；但 Client 不自动重放原 interaction/request。新的 action 使用新的 interaction context 和新 generation。

S0 不存在 generation grace window。

## 8. Resource / route / SSRF boundary

Relay 根据当前 control state 做：

```text
resourceId
→ server-only route
→ active upstream or private Gateway target
→ method/path/transport policy
```

Path 安全要求：

- runtimePath 为 path-only；
- 拒绝 absolute URI、scheme/host/userinfo override；
- 拒绝 traversal 与等价编码绕过；
- 必须满足 route allowlist；
- query 可以透明传输但不能改变 target host；
- shared Upstream 的 management/internal endpoint 不能借 resource path 到达；
- S0 不执行 generic path rewrite DSL。

需要 provider path/body/query conversion 时由 Upstream Adapter 负责。

## 9. Header / credential policy

Outbound 必须清除/覆盖 client 可能伪造的：

- enterprise `Authorization`；
- Host/hop-by-hop；
- platform internal/correlation headers；
- route/security policy 禁止的 header。

随后 Relay 使用 current server-side Upstream config 注入真正 runtime credential 和自身 request correlation。

Gateway target 例外仅在 credential 类型和 Principal envelope：Relay 清除客户端伪造的所有 internal identity header，注入 dedicated service credential、request correlation 与完整性保护的 Principal envelope；它仍不得读取/改写 MCP JSON-RPC body。

Response side 至少保证：

- internal credential 不回显；
- hop-by-hop 语义正确；
- `Set-Cookie` 不形成 provider cookie 越界到 Client；
- redirect 不能让 Client 绕出 Relay 安全边界；
- provider 4xx/5xx 不被错误包装成 Relay success。

精确 HTTP 行为/Problem code 以 Control Protocol 为准。

## 10. Transparent transport

S0 Relay 必须透明承载：

### HTTP request/response

method、path/query、content type、body 和 compatible response 不做 provider business rewrite。

### Streaming / SSE

- progressive flush；
- 不聚合完整响应；
- 不解析/重写 provider SSE business data；
- client disconnect/cancel 传播 upstream。

### Binary response

TTS 等 binary bytes 保真，不转码、不整体 buffer。

### Multipart / upload

ASR 等 multipart/file request 以 streaming-friendly 方式转发，并在不整体读入内存的前提下执行 size/security limits。

### MCP Streamable HTTP

Relay 只执行 HTTP/security/credential boundary，不解析/改写 MCP JSON-RPC business semantics。

对 `twg_*` Gateway resource，Relay 也只做相同透明传输：不知道 `discover_tools` query、`toolRef`、真实 tool name 或 arguments。Gateway tool-level error/result 不能被 Relay重写成另一套业务协议。

实时 ASR 使用显式 WEBSOCKET 路由与 GET 升级；握手前执行同一 admission 链，普通 HTTP 路由拒绝 Upgrade。透明保留音频帧/转写事件，按连接记录 HTTP 101、时长和双向传输字节，不能伪造语义识别时长。字节计量不含 HTTP 握手头；请求字节上限约束单连接累计客户端到上游的帧传输字节，并非单帧大小。连接遵守 idle/overall 限制，关闭任一端或取消须关闭另一端。详见 Control Protocol §10.6。

## 11. Timeout / cancellation / retry

- cancellation 必须沿 request context 传播到 upstream；
- long-running stream 不应被统一短 timeout 误杀；
- request/body/header 等 resource limit 在 architecture-defined安全边界内生效；
- Relay **不自动重试任何已经开始 forward 的可能有副作用的 Runtime request**；
- retry 只用于明确有幂等契约的 control apply / usage delivery 等内部流程。

具体 timeout 值、transport/client implementation 由 core 仓库维护。

## 12. Request Usage 与 durable spool

Relay 对 accepted/blocked Runtime request 形成 Control Protocol §18 规定的 request-level facts；只有可信 Principal 成立才进入用户 Usage，未解析的 route/Upstream 不伪造。Gateway target 无普通 Upstream。至少能关联：

```text
request identity
interaction/principal when available
validated resource / resolved internal route / typed target when available
managed generation / control revision
start/end/status/forwarded/duration
request/response size facts where allowed
stable error classification
```

Relay 不猜 token、TTS character、ASR duration 等 provider-specific semantic usage。

Durable delivery：

```text
request completes/fails
→ persist request fact to local durable spool
→ background batch delivery to Hub
→ Hub accepted/duplicate ACK
→ delete acknowledged rows
```

只在 spool durable write 成功后承诺 at-least-once。disk/write failure 必须进入可观察的 metering degraded 状态；Hub outage 不得静默丢弃已成功 spool 的事件；poison row 不能永久阻断所有正常 row。

具体 SQLite schema/pragma/batch loop/backoff implementation 属于 core implementation。

## 13. Auth / revoke

Relay 只信任 Control Protocol 定义的 Hub-issued enterprise access token 和当前 control state 中的 public verification/restrictive principal state。

验证必须覆盖 token authenticity/claims/deployment/audience/time + User/Device/Session restrictive state。

Relay 不向 Hub做每请求 introspection。Security revoke 由 Hub先提交 authority，再通过新 control revision推进 Relay；Relay ACK 后完全 enforce。

## 14. Internal topology

Public Runtime surface 与 Hub-only control surface 必须具有真实网络/trust separation：

- `/internal/*` 不经 public ingress；
- internal service identity 与 User/Admin credential 分离；
- cross-host internal traffic 使用 secure transport；
- Relay→Gateway 使用独立 scoped service identity，Gateway private listener 不接受 public Android bearer 直连；
- public caller 不能借 runtime route 调用 Relay management/control endpoint。

是否使用一个或两个 listener、具体 listen address/env 名由 core deployment implementation 决定，只要上述边界成立。

## 15. Health / observability

至少区分：

```text
process live
runtime ready (valid current control loaded)
applied control revision/hash
active generation
usage spool health/backlog
version/build/uptime
```

Status/log 不暴露 Upstream credential、private key 或敏感 payload。

## 16. Resource / shutdown principles

S0 目标是轻量、可预测：

- runtime request path 无 Hub DB/ORM query；
- control state 是 immutable read-mostly view；
- request/response 不整体 buffer；
- background subsystem 数量最小化；
- concurrent stream/resource growth 可测、可回落；
- graceful shutdown 停新流量、bounded drain、cancel 超时 request，保留已 durable spool 的 Usage。

不为了虚假峰值 QPS 提前引入 broker、worker pool、local control cache 或通用 gateway framework。

## 17. 明确非目标

S0 Relay 不实现：

1. PREPARE/BARRIER/COMMIT/ABORT state machine；
2. durable full control-state/Secret cache；
3. Provider protocol/body translation；
4. 实时 ASR 之外的通用 WebSocket 业务框架；
5. Redis/Kafka/message broker；
6. client-visible runtimeRouteId；
7. per-user/group capability ACL index；
8. Gateway Catalog/search/toolRef/schema/downstream MCP business logic；
9. generic quota engine；
10. generic path rewrite DSL；
11. automatic retry of forwarded Runtime request；
12. complex background Upstream health scheduler。

## 18. Component Exit

Runtime Relay 合格必须由 `measix-s0-runtime-relay-testing-spec.md` 和对应 System Testing Spec 证明至少：

1. full control state apply 原子、幂等、可 status/reconcile；
2. restart 前后 fail-closed/rehydrate 正确；
3. auth/generation/resource/path failure 全部 no-forward；
4. Model streaming、TTS binary、ASR multipart、Direct/Gateway MCP Streamable HTTP 透明传输；
5. cancellation 和 long stream 不出现系统性泄漏/错误 buffering；
6. credential/header/redirect/cookie/SSRF boundary 正确；
7. request usage durable spool/replay/dedupe/degraded observable；
8. request capture 不因并发 control apply 中途漂移；
9. Gateway route/service credential/Principal envelope 可验证，Relay 不解析 discover/invoke；
10. Relay 始终 provider/tool-business-agnostic；
11. implementation repository 可以自由改变内部 proxy/spool/package 设计而不破坏上述 architecture invariants。
