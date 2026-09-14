# S0 Upstream Adapter Integration Contract

> 状态：S0 External Integration Contract  
> 版本：2026-08-31
> 上位文档：`measix-s0-foundation-contract-spec.md`  
> S0.1 required profile：`measix-s0-capability-delivery-contract-spec.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> Qualification：`measix-s0-upstream-adapter-qualification-spec.md`  
> 文档职责：定义外部 AI/Speech/MCP Upstream Adapter 的协议、网络、传输、Correlation、Usage 与安全兼容边界；具体运行认证字段以 Control Protocol 为唯一权威。

## 1. Role

Upstream Adapter 负责 Provider/protocol compatibility：

```text
Client compatible payload
→ Runtime Relay transparent L7
→ Upstream Adapter
→ actual AI/Speech/MCP provider
```

Adapter 不拥有 EnterpriseUser、Device、ManagedPolicy、Release、Snapshot 或 EnterpriseSession。

通用 ERP/CRM/DB/SaaS 多协议连接、业务写入/审批/补偿属于后续 Enterprise Connector，不因底层使用 MCP 而混入本 Adapter 角色。S0.3 可以通过 Enterprise Tool Gateway 接入受信、经审核的只读 downstream MCP 企业查询；其来源、Catalog、授权和执行合同由 Gateway specs 管理，不归本 AI/Speech/Direct MCP Adapter 兼容层，也不因此提前建设通用 Connector 平台。

## 2. Brand independence

Relay 不允许出现 Adapter 品牌分支。Adapter-specific compatibility/semantic usage connector 只能位于 Control Hub 的 external integration boundary。

## 3. Identity boundary

Platform identity：

```text
mdl_* / tts_* / asr_* / mcp_*
ups_*
rte_*
req_*
```

External opaque values：

```text
upstreamModelKey
providerRequestId
sourceEventId
```

外部值不能替代平台 stable identity。

## 4. Upstream operational state

Upstream candidate 与 active state 分离：

```text
upstreamId
configRevision
activeConfigRevision?
status
candidateConfig
```

Save candidate 不改变 runtime；Apply + Relay ACK 后才更新 active revision。

Base URL 只能由 Admin operational config 提供；Runtime Client 不可指定或覆盖 scheme/host/port。

## 5. Transport capability

```text
HTTP_REQUEST_RESPONSE
HTTP_STREAMING_SSE
HTTP_BINARY_STREAM
HTTP_MULTIPART
```

S0 不支持 Runtime WebSocket tunnel。

## 6. Protocol vocabulary 与 release support 分离

通用兼容 vocabulary 可以包含：

```text
OPENAI_CHAT_COMPLETIONS
OPENAI_RESPONSES
ANTHROPIC_MESSAGES
OPENAI_AUDIO_SPEECH
OPENAI_AUDIO_TRANSCRIPTIONS
MCP_STREAMABLE_HTTP
```

但是 **S0.1/S0.4 required and VERIFIED release baseline** 只有：

```text
OPENAI_CHAT_COMPLETIONS
OPENAI_AUDIO_SPEECH
OPENAI_AUDIO_TRANSCRIPTIONS
MCP_STREAMABLE_HTTP
```

因此：

- enum 存在不代表当前产品已支持；
- Admin 正常 authoring 只暴露已实现/qualification 的 profile；
- future Responses/Anthropic/Google-native 等 profile 需要 client compatibility + qualification；
- unknown/unimplemented protocol 不允许 silent fallback。

## 7. Required reference profile

### Model

Adapter 接受 OpenAI Chat Completions-compatible request；如果资源声明 streaming，则支持 compatible streaming/SSE response。Relay 不做 provider body translation。

### TTS

Adapter 接受 OpenAI-compatible Audio Speech semantics：

```text
model
input
voice
response format = MP3 for S0.1 required profile
```

返回 binary audio；Relay 不转码。

### ASR

Adapter 接受 OpenAI-compatible Audio Transcriptions multipart：

```text
file
model
optional language
```

返回 compatible transcription result。Realtime/WebSocket 不属于 S0 required profile。

### MCP

MCP 使用标准 Streamable HTTP；不套用 OpenAI provider semantics。

## 8. Runtime path

Client：

```text
/runtime/v1/resources/{resourceId}{runtimePath}
```

Relay 根据 compiled resource→route→upstream state 选择 active Adapter endpoint。No generic path rewrite DSL；需要 path/body/query conversion 时由 Adapter 负责。

`runtimePath` 为 normalized path-only 且必须落在 route allowlist；Client 不可覆盖 host/scheme/port。

## 9. Runtime authentication boundary

Upstream runtime authentication 由 Control Hub operational config 与 Runtime Relay 内部控制状态负责，具体字段以 Control Protocol 为权威。

核心不变量：

- Android/Test Client 只携带 Platform Runtime identity；
- Adapter/provider runtime credential 不进入 Client Snapshot；
- Relay 在 server-side 注入 Adapter 所需运行认证；
- Provider-specific OAuth/refresh lifecycle 如需要，由 Adapter 自身管理；
- runtime credential 不进入 status/usage/report/client response。

## 10. Managed MCP auth ownership

S0.1 Managed MCP 只支持：

```text
ENTERPRISE_MANAGED
NONE
```

`USER_MANAGED` Managed MCP 延后到 per-user delegated credential transport 被正式设计之后。Android Local MCP OAuth 仍是 Local capability。

## 11. Header / redirect / cookie

Platform runtime identity 不应被 Adapter 当作 provider credential。

Correlation 首选平台 `req_*` request ID header。Adapter 不应把服务端运行认证信息回显给 Client。

S0 正常 provider request 不依赖 client-side redirect；Relay 按 Control Protocol 处理 redirect/cookie 边界。

## 12. Streaming / binary / multipart

Streaming：progressive flush、no unnecessary buffering、client cancellation propagation where supported。

TTS：binary integrity/content type preserved。

ASR：multipart field/file/content type preserved within configured limit。

MCP：Streamable HTTP request/session/stream semantics preserved within Relay security policy。

## 13. Connection Test vs Qualification

Connection Test 只证明 candidate endpoint reachability/basic probe。

Qualification 才证明：

```text
specific upstream/config revision
+ adapter version
+ protocol profile
+ required transport
+ correlation/usage level
```

Known-incompatible required capability → Publish hard error；unverified but plausible capability → warning according to validation policy。

## 14. Correlation capability

Supported declaration examples：

```text
HEADER_ECHO
VIRTUAL_KEY
REQUEST_LOG_ID
USAGE_API
WEBHOOK
NONE
```

无法稳定映射 provider event 到 platform request 的能力不能宣称 request-level semantic usage。

## 15. Usage capability level

```text
LEVEL_0  Relay request facts only
LEVEL_1  aggregate semantic usage
LEVEL_2  request-correlated semantic usage
```

平台不通过 byte/log heuristic 将能力级别向上猜测。

Adapter-specific semantic connector 位于 Hub integration boundary；公共 UpstreamConfig 不做通用 provider-usage DSL。

## 16. Standard semantic meter

S0.1 required vocabulary：

```text
INPUT_TOKENS
OUTPUT_TOKENS
CACHED_TOKENS
CHARACTERS
AUDIO_SECONDS
REQUESTS
```

Mapping：

```text
MODEL → token meters + requests
TTS   → characters/audio seconds/requests
ASR   → audio seconds/requests
MCP   → requests
```

No reliable semantic source → PARTIAL/UNKNOWN，禁止 bytes→token/audio 猜测。

Provider extension meter 使用 namespaced key，不覆盖标准含义。

## 17. Failure / cancellation

Adapter qualification 必须验证 connection failure、timeout、provider 4xx/5xx、stream cancellation、binary/multipart cancellation。

Platform timeout 不保证 provider 已停止实际执行/计费；如有 provider usage report，以真实来源为准。

## 18. Security boundary

- Adapter management endpoint 不通过 Resource route 暴露；
- allowed paths 只包含业务 endpoint；
- runtime request 不能改变 Upstream host；
- unsafe path traversal/absolute target 在 Relay 边界拒绝；
- logs/reports 不保存运行认证材料或真实敏感 prompt；
- private Adapter management port 不由 platform ingress 公开。

## 19. Qualification requirement

新 Adapter/profile 标记 VERIFIED 前至少验证：

```text
connection/basic compatibility
normal request/response
required transport
stream/cancel when applicable
provider 4xx/5xx behavior
path/header/security boundary
TTS binary if claimed
ASR multipart if claimed
MCP Streamable HTTP if claimed
correlation if claimed
semantic usage/cost if claimed
no credential leakage
```

S0.1 Release 对每个声称 VERIFIED 的 required profile 都必须有证据。不同 profile 可以来自不同 qualified Adapter/endpoint；不要求单一供应商覆盖四类能力。

## 20. Compatibility expansion

S0.1 之后增加 native Provider profile：

1. define clientProtocol semantics；
2. prove Client/Android mapping；
3. update executable fixtures；
4. qualify protocol/transport；
5. expose Admin option after support；
6. preserve frozen Snapshot compatibility or introduce explicit schema version。

新增 profile 不得向 Relay 引入 Provider brand/body translation，也不得在 required S0 profile 已完成后阻塞进入 S1。
