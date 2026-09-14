# S0.1 Upstream Adapter Qualification 规格

> 状态：S0.1 External Integration Qualification Baseline  
> 版本：2026-08-20  
> 外部契约权威：`measix-s0-upstream-adapter-contract.md`  
> S0.1 Contract：`measix-s0-capability-delivery-contract-spec.md`  
> Relay：`measix-s0-runtime-relay.md`  
> Hub：`measix-s0-control-hub.md`  
> 文档职责：定义某 Adapter/endpoint/profile 被标记 VERIFIED 前必须如何配置、测试、记录证据；不定义 Adapter 品牌私有实现。

## 1. Qualification 的单位

不再使用模糊的“某 Adapter 支持 S0”。Qualification 单位必须是：

```text
adapterName/version
+ upstreamId/configRevision
+ clientProtocol profile
+ transport
+ correlation/usage capability
```

一个 Adapter 可以只对 Model VERIFIED，TTS/ASR 为 UNVERIFIED；Admin/Release claim 必须按 profile 展示真实状态。

## 2. S0.1 required profiles

```text
OPENAI_CHAT_COMPLETIONS
OPENAI_AUDIO_SPEECH
OPENAI_AUDIO_TRANSCRIPTIONS
MCP_STREAMABLE_HTTP
```

`OPENAI_RESPONSES`、`ANTHROPIC_MESSAGES` 等 future compatibility profile 不属于 S0.1 Exit requirement；如果额外支持，独立 qualification，不借 required profile 结果推断。

S0.1 overall release 至少要为它声称可用的四类 Managed Capability 准备真实 qualification evidence；允许不同 profile 使用不同 endpoint/Adapter。

## 3. Test profile

```text
AdapterQualificationProfile
  adapterName
  adapterVersion
  upstreamId
  configRevision
  declaredClientProtocol
  declaredTransport
  correlationMode
  requestedUsageLevel
  testResource
    resourceKind
    upstreamModelKey?
    runtimePath
    non-sensitive request fixture reference
```

运行认证由 test environment 安全注入，不写 profile/report。

## 4. Result

```text
QualificationResult
  status VERIFIED|FAILED|PARTIAL
  adapterVersion
  testedConfigRevision
  testedProfile
  verifiedTransport
  verifiedCorrelationMode
  verifiedUsageLevel
  startedAt/completedAt
  reportHash
  findings[]
```

不创建新的 long-term `qual_*` entity ID；结果关联 `upstreamId + configRevision + adapterVersion + profile`。

Config revision、Adapter major version、relevant protocol behavior 变化后 verification 失效/需重跑。

## 5. Common suite

每个 profile：

1. reachability/basic probe；
2. runtime authentication success/failure boundary；
3. normal request/response；
4. provider 4xx/5xx behavior；
5. path allowlist / management endpoint isolation；
6. redirect/cookie/header behavior；
7. request correlation where claimed；
8. cancellation；
9. timeout；
10. report/log no runtime credential or sensitive content。

## 6. Model — Chat Completions

`QUAL-MDL-*`：

- compatible request shape accepted；
- configured `upstreamModelKey` works；
- normal response compatible；
- streaming/SSE progressive output；
- cancellation observed；
- tool/reasoning/vision capability only marked VERIFIED when corresponding fixtures succeed；
- no Relay body translation required。

Model basic streaming is mandatory for S0.1 real qualification。

## 7. TTS — Audio Speech

`QUAL-TTS-*`：

- compatible speech endpoint；
- model + input + **voice** semantics honored；
- S0.1 MP3 response profile；
- binary content type/integrity；
- cancellation/timeout；
- semantic characters/audio duration only claimed if trustworthy source exists。

If an endpoint does not support Audio Speech, it cannot be used for S0.1 Managed TTS even if its model API is otherwise OpenAI-compatible。

## 8. ASR — Audio Transcriptions

`QUAL-ASR-*`：

- HTTP multipart endpoint；
- file + model preserved；
- optional language when configured；
- compatible transcription result；
- large-body/cancellation behavior；
- audio duration semantic usage only claimed if reliable；
- no WebSocket requirement。

## 9. MCP — Streamable HTTP

`QUAL-MCP-*`：

- standard Streamable HTTP endpoint；
- initialize/basic discovery/tool call path according to current MCP contract；
- content/session/stream behavior；
- cancellation；
- `ENTERPRISE_MANAGED` or `NONE` server-side runtime auth boundary；
- no requirement for Android user-managed OAuth in S0.1。

## 10. Correlation qualification

根据声明验证：

```text
HEADER_ECHO
VIRTUAL_KEY
REQUEST_LOG_ID
USAGE_API
WEBHOOK
NONE
```

LEVEL_2 必须能稳定把 semantic event 映射到 `req_*`。只能 aggregate 到 model/resource/time → 最大 LEVEL_1。不能 correlation → LEVEL_0。

## 11. Semantic Usage qualification

For claimed semantic usage：

```text
platform requestId
↔ provider/adapter source event
↔ normalized meter
↔ completeness
↔ optional provider cost/currency
```

Required standard meters according to profile：

```text
MODEL INPUT_TOKENS / OUTPUT_TOKENS / CACHED_TOKENS / REQUESTS
TTS   CHARACTERS / AUDIO_SECONDS / REQUESTS
ASR   AUDIO_SECONDS / REQUESTS
MCP   REQUESTS
```

Samples include success/error/cancel/stream and cached-token case if claimed。

Failed reconciliation must downgrade capability；never compensate with byte heuristics。

## 12. Security / SSRF suite

- Runtime Client cannot change scheme/host/port；
- path traversal/absolute URL denied by Relay；
- management endpoint not in allowed business paths；
- platform runtime identity not exposed as provider credential；
- unsafe client headers cannot override server policy；
- streaming compression/buffering preserves semantics；
- report contains no runtime authentication material or sensitive prompt/body。

## 13. Connection Test vs Qualification UI

Admin Upstream can display：

```text
Connection Test PASS/FAIL
Qualification UNVERIFIED/VERIFIED/FAILED/PARTIAL
Adapter Version
Profile
Transport
Correlation
Usage Level
Last Verified
Findings
```

Connection Test is not qualification。Saving a candidate does not magically retain verification for a changed revision。

## 14. Machine-readable artifact

Report contains：

```text
adapterName/version
upstreamId/configRevision
profile
transport
status
correlationMode
usageLevel
scenario results
durations
findings
reportHash
timestamps
```

No runtime credential/full sensitive business content。

S0.1 Freeze manifest references the relevant report(s) by stable artifact hash/path/CI reference。

## 15. S0.1 Exit condition

For each capability that S0.1 release claims VERIFIED：

- exact profile VERIFIED；
- required transport VERIFIED；
- path/security suite passes；
- runtime authentication boundary passes；
- correlation/usage level matches evidence；
- known deviations documented；
- report attached/referenced in S0.1 freeze manifest。

At minimum Model Chat Completions streaming must have real Adapter evidence；S0.1 required TTS/ASR/MCP release claim also needs corresponding real endpoint/profile evidence before final S0.1 Freeze。

## 16. Compatibility expansion

Later native Provider/profile support uses the same qualification model. A previous Adapter-level “VERIFIED” result is never automatically inherited by a new clientProtocol or transport。
