# S0 Enterprise Tool Gateway 架构与需求规格

> 状态：S0 Component Architecture / 组件架构基线
> 版本：2026-08-31
> 上位合同：`measix-s0-enterprise-tool-gateway-contract-spec.md`
> 协议权威：`measix-s0-control-protocol.md`
> Hub/Relay 边界：`measix-s0-control-hub.md`、`measix-s0-runtime-relay.md`
> 测试：`measix-s0-enterprise-tool-gateway-testing-spec.md`
> 文档职责：定义 Enterprise Tool Gateway 的职责、applied state、MCP/downstream/security/execution/metering/recovery 不变量；不定义具体 package/file、数据库 DDL、检索库、连接池或运行配置名。

## 1. 正式定位

Enterprise Tool Gateway 是 Enterprise Realm 的受治理工具运行面。它把 Control Hub 已审核发布的 Tool Catalog 投影为固定的两个模型工具，并代理调用 platform tool 或 downstream MCP。

它回答：

1. 当前 generation 的固定 Gateway surface 是什么；
2. 当前 Principal 可发现哪些已发布只读工具；
3. 某个 `toolRef` 是否仍对当前 interaction 有效；
4. arguments 是否符合已发布 schema；
5. 应调用哪个 platform adapter/downstream session；
6. 真实工具执行产生了哪些审计/用量事实。

Gateway 不是 Managed Release authority、Identity authority、Android Conversation owner、通用 Connector framework 或 Runtime Relay。

## 2. 三进程边界

```text
Control Hub
  ├─ Draft/Release/Profile/Candidate/Published Catalog authority
  ├─ Gateway desired control / status reconciliation
  └─ Enterprise Update private typed read authority

Runtime Relay
  ├─ public auth/principal/generation/resource admission
  ├─ private route/service credential/principal envelope
  └─ request-level metering

Enterprise Tool Gateway
  ├─ standard MCP initialize/tools/list/tools/call
  ├─ discover/invoke semantics
  ├─ applied Published Catalog/search index
  ├─ toolRef validation/schema enforcement
  └─ downstream MCP/platform tool execution facts
```

禁止依赖方向：

- Gateway 不访问 Hub DB/ORM/domain persistence；
- Relay 不 import Gateway Catalog/search/tool domain，也不解析 MCP JSON-RPC business body；
- Hub 不代理通用 downstream MCP body/stream；
- Gateway 不验证 Android bearer 或自行读取 User/Device authority；
- Gateway downstream credential、private endpoint 和 applied state 不进入 Android Snapshot/log/tool result。

## 3. GatewayControlState

Gateway 的唯一运行 authority 是 Hub 下发并成功应用的完整 immutable `GatewayControlState`，语义至少包含：

```text
gatewayControlRevision + bundleHash
deploymentId
surface @ managedGeneration
published catalogs @ managedGeneration
integration endpoint/protocol/resolved credential/limits
platform tool route policy
verification/signing material needed for toolRef
operational limits
```

Apply 顺序：decode→完整引用/schema/security/endpoint validation→构造 canonical surface/catalog/index/downstream plan→原子替换 current state→ACK exact revision/hash。

same revision/same hash 幂等；stale revision 或 same revision/different hash 拒绝。任何失败保持旧 state 完整可用，不允许逐 catalog/integration 暴露半新半旧状态。

Gateway-first/Relay-second handoff 的完整 state 最多保留 Relay-current + next-staged 两个 generation；这只保障 Relay 尚未切换时的当前流量和已接受在途请求。Gateway 按受信 Principal envelope 精确选择 generation，不做 latest/old fallback；Relay 切换后旧 interaction 的下一次请求在 Relay 得到 428，Hub 随后用新 control revision 清理不可达旧 generation。

每个 accepted request capture immutable applied view；并发 apply/cleanup 只影响后续 request。S0 不使用客户端可达的旧 generation grace window。

## 4. 启动、重启与 rehydrate

Gateway 不把完整 control/credential 持久化为独立 authority：

```text
process start
→ no valid GatewayControlState
→ MCP runtime fail closed
→ private health/control available
→ Hub rehydrates desired state
→ validate + atomic apply
→ READY
```

Hub 短时不可达但进程未重启时，可以继续使用已应用 state。restart 后不能从未受 Hub 确认的 stale disk copy 恢复运行。Candidate/Published truth 始终在 Hub；Gateway index/session/cache 都可重建。

## 5. MCP surface

Gateway 实现标准 MCP Streamable HTTP server boundary。对 authorized generation：

- `tools/list` 只返回 `discover_tools`、`invoke_tool`，按固定顺序；
- 两个定义由 applied canonical surface 产生；
- 不把真实 Published Catalog 作为顶层 MCP tools 暴露；
- `tools/call` 只接受这两个名称，其他名称返回 typed unknown-tool；
- `tools/list_changed` 不用于把 Candidate drift 推给客户端；只有新 Managed generation 激活后，新的 interaction 才消费新 surface。

Snapshot `surfaceHash` 是客户端对标准 `tools/list` 的期望摘要，不是替代 `tools/list` 的工具定义副本。摘要输入/算法只由 Control Protocol §10.8 定义；Gateway、Hub compiler 与 Client 必须使用同一组 canonical fixtures。

## 6. Catalog search

Gateway 只查询 captured generation 的 Published Catalog，并先应用 authorization/enabled/risk filter，再 ranking。不得先从全量目录检索再在结果末尾删权限，因为 timing/count/ordering 也可能泄露未授权能力。

搜索满足有界、确定、可解释和 stable tie-break。具体 lexical/FTS/BM25/embedding/rerank 组合属于实现，但必须通过同一 evaluation corpus；不可因外部 embedding/LLM 不可用让 exact-name/alias baseline 失效。

## 7. `toolRef` 与 invocation

Gateway 签发并验证短期 opaque `toolRef`，至少绑定 deployment/user/device/session/interaction/generation/tool/schema/expiry。它是 capability reference，不是数据库实体、可枚举 tool name 或长期 token。

调用链：

```text
verify trusted Principal envelope
→ verify toolRef signature/bindings/expiry
→ resolve captured Published Gateway Tool
→ re-check enabled/READ_ONLY/current authorization
→ verify schemaHash
→ validate arguments against authoritative sourceInputSchema
→ select platform adapter or downstream MCP integration
→ execute once under timeout/cancellation policy
→ validate/normalize result boundary where contract requires
→ record resolved tool execution facts
```

Gateway 不自动重试已开始的 downstream tool call。timeout/network uncertainty 返回 outcome unknown where appropriate；只有 downstream 明确证明幂等且上位合同允许时，未来阶段才可增加 retry policy。

## 8. Downstream MCP lifecycle

Gateway 是 downstream MCP client，负责 initialize/capability negotiation、`tools/list`/list-changed subscription、session/transport lifecycle、timeout/cancellation 和 protocol error normalization。

Candidate refresh 与 runtime invocation 分离：远端 list-changed 只触发采集/diff 上报 Hub，不更新 applied Published Catalog。Runtime invocation 始终用 published sourceName/schemaHash；若远端工具缺失或 schema drift，fail closed、记录 drift，不以最新 Candidate 偷跑。

S0.3 只支持 MCP Streamable HTTP 和企业共享 server credential。per-user OAuth/elicitation、stdio、SSE legacy transport、REST/DB connector 不在范围内。

## 9. Platform tool adapter

`get_enterprise_updates` 通过专用 typed adapter 调 Hub private read API。该 adapter：

- 只允许已定义的 read operation/arguments；
- 不调用 Admin mutation surface；
- 不读取 Hub DB；
- 使用 Gateway service identity + trusted Principal context；
- 与 Portal/Client Feed 共用 Enterprise Update authority/filtering semantics；
- 不扩张为任意 Hub endpoint proxy。

## 10. Identity、credential 与网络

Gateway 只接受来自 Relay 的 private TLS/service-authenticated request。Relay 必须删除 client-supplied internal principal headers，再注入签名或等价完整性保护的 Principal envelope。

Gateway 验证 envelope 的 issuer/audience/time/request/deployment/generation，并把 identity 作为 authorization、toolRef 和 execution fact 输入。它不信任 MCP body/header 中的同名字段。

Downstream 使用 Gateway applied state 中的 enterprise credential。Android token、Hub Admin session、raw Managed Secret、service signing key 不向 downstream 或模型传递。Redirect、cookie、header、SSRF/path 边界至少与 Relay 对 Upstream 的安全等级等价。

## 11. Result、UI metadata 与不可信内容

真实工具结果按 MCP content/structuredContent 语义返回，但始终视为不可信外部内容。Gateway 不把 downstream 返回的 prompt/instruction 提升为平台指令，也不记录完整敏感 payload。

Gateway 同时产生 client-safe resolved execution metadata（真实 agent-facing name、gatewayToolId、status、request correlation），供 Android S0.4 的工具卡展示。该 metadata 使用协议保留的 client metadata/correlation 边界，不要求模型从任意结果文本反解析，也不暴露 source endpoint/credential。

S0.3 不实现用户审批；由于只允许 READ_ONLY，UI 仍必须清晰显示工具已被暴露和调用。写/破坏性工具进入后续阶段前必须先定义 confirmation/approval/idempotency/unknown-outcome contract。

## 12. Metering、Audit 与 diagnostics

Relay 继续拥有 public runtime request fact；Gateway 产生 tool-level execution fact，至少关联：

```text
requestId / interactionId
deploymentId / userId / deviceId when present
managedGeneration
gatewayToolId / sourceKind / toolIntegrationId when downstream (platformToolName otherwise)
resolved agent-facing name + source identity hash
started/completed/status/duration/forwarded
schemaHash / catalogHash
stable error class
```

Gateway Tool fact 与 Relay RequestUsage 不互相冒充，Hub 以 request correlation 汇总。S0.3 不因缺少 downstream semantic meter 伪造 cost。日志/diagnostics 不记录 tool arguments/result body/credential，除非未来独立 retention/redaction policy 明确定义。

平台工具不伪造 ToolIntegration；Relay 记录的 Gateway resource target 也不伪造 Upstream。精确 source/target 条件字段以 Control Protocol 为准。MCP 工具与 annotation/result 边界参考[官方 tools 规范](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)；实际支持的协议版本与 schema 能力由实现/qualification 固定，不能把第三方 annotation 当成安全证明。

三个 daemon 的生产监管与通用结构化日志字段以 S0 Implementation Decision 为权威，具体 units/配置/保留/runbook 归 platform-core Operations。Gateway 必须复用共同日志初始化并附带 `service=enterprise-tool-gateway`、`buildVersion`、stable `event`；适用时增加 request/interaction/generation/gatewayControl/tool correlation。它不另建私有滚动日志、日志采集 daemon 或 Admin 原始日志浏览面。

## 13. 明确非目标

Gateway S0.3 不实现：Managed Release/Identity durable truth、public ingress、Direct MCP fallback、write/destructive tools、user approval、per-user OAuth/RBAC、generic connector/workflow/code sandbox、provider model translation、Gateway HA、durable full control cache、必选 embedding/Router LLM。

## 14. Component Exit

Gateway 组件合格必须证明：

1. control apply 原子、幂等、restart fail-closed、rehydrate/reconcile 正确；
2. standard MCP surface 固定且与 Snapshot surfaceHash 一致；
3. Catalog filter-before-rank、deterministic search、Candidate/Published 隔离正确；
4. toolRef/schema/Principal/generation/expiry 全部 fail closed；
5. downstream MCP lifecycle/drift/cancel/timeout/unknown outcome 正确；
6. platform Update adapter 与 external MCP 都不越过 Hub/Relay authority；
7. credential/header/redirect/cookie/SSRF/result/log boundary 正确；
8. tool-level facts 可与 Relay request correlation，且不泄漏敏感 payload；
9. 实现可以更换内部 index/session/package 设计而不改变上述 architecture invariants。
