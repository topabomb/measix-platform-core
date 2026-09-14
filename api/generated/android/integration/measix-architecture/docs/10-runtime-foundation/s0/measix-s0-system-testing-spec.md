# S0 System Testing 规格

> 状态：S0 System Testing Authority / Final Stage Verification Baseline  
> 版本：2026-08-31
> 上位范围：`measix-s0-foundation-contract-spec.md`
> S0.1 Gate：`measix-s0-capability-delivery-system-testing-spec.md`
> S0.2 Gate：`measix-s0-enterprise-realm-experience-testing-spec.md`
> S0.3 Gate：`measix-s0-enterprise-tool-gateway-testing-spec.md`
> S0.4 Contract：`measix-s0-android-integration-contract-spec.md`
> Wire 权威：`measix-s0-control-protocol.md`  
> 实施权威：`measix-s0-implementation-decision.md`  
> 配套：component testing specs + upstream adapter qualification  
> 文档职责：定义 S0 **最终**跨仓库 T4/RC/Exit。S0.1 Test Client、S0.2 A/B/C/Portal proof 与 S0.3 Gateway Test Client proof 都是前置 Gate，不替代本文件要求的真实 Android emulator/device 全 profile 验证。

## 1. S0 最终测试目标

S0 只有三个闭环都真实成立才完成：

```text
Admin Console
  → Control Hub
      ├─ Gateway control / Catalog → Enterprise Tool Gateway ACK
      └─ Runtime control → Runtime Relay ACK
  → ACTIVE Release / Snapshot / Usage / Diagnostics

Test Client
  → Runtime Relay
  → Enterprise Tool Gateway
  → discover_tools / invoke_tool
  → platform tool adapter or downstream MCP
  → audited result / Usage correlation

Android Client
  → Personal/Enterprise ClientRealm boundary
  → Control Hub preflight / Snapshot
  → Runtime Relay
  → Upstream Adapter or Enterprise Tool Gateway
  → Usage correlation
```

Final tests answer：

1. component correctness；
2. executable contract compatibility；
3. Publish/security/generation/usage cross-process convergence；
4. Admin can really author required capabilities；
5. Gateway can really expose only the frozen two-tool surface and execute the reviewed Published Catalog；
6. Android can really consume frozen S0.3 Snapshot v5 inside Enterprise Realm, while Personal Realm remains free of Managed content；
7. required Model/TTS/HTTP-ASR/Direct MCP/Gateway profile works end-to-end；
8. failure/restart/security does not violate invariant。

## 2. Stage Gate relationship

```text
S0 Core CI
  ↓
S0.1 CAP Gate
  → server-side product contract complete
  → 当前 Snapshot 资源协议与实际 schemaVersion 固定
  → freeze manifest
  ↓
S0.2 Enterprise Realm & Experience Gate
  → Snapshot schemaVersion=4 frozen
  → A/B/C + Portal MVP product proof
  → S0.2 freeze manifest
  ↓
S0.3 Enterprise Tool Gateway Gate
  → Snapshot schemaVersion=5 frozen
  → server/Admin/Test Client two-tool proof
  → S0.3 freeze manifest
  ↓
S0.4 Android Component/Emulator/Device Gate
  ↓
S0 Final RC Gate (this document)
```

任一 sub-stage pass 都不等于 S0 pass；S0.4 component pass 也不等于 final S0 pass。

Final RC 的唯一字段清单见 §25；必须 pin 每个子阶段 Freeze 与真实 server/Admin/Android/Portal composition。No floating “latest main” pair，不维护第二份字段别名清单。

## 3. Test layers

| Layer | Purpose | Allowed substitutes |
|---|---|---|
| T0 Static/Contract | schema/codegen/fixture/migration/build | no runtime components |
| T1 Unit/Domain | pure state/validation/mapping | clock/random/IO adapter |
| T2 Component Integration | one component + real local boundaries | controlled peer fake |
| T3 Cross-component | multiple real MEASIX processes | external Adapter may be deterministic |
| T4 System/E2E | Admin + Hub + Relay + Gateway + Android + client-facing topology | deterministic Adapter for repeatability; RC also real qualification |

Rules：

- SQLite/HTTP/streaming/persistence critical boundaries are real；
- no fixed large sleep for correctness；
- no flaky critical test retry-until-green；
- no line-coverage percentage substitutes required scenarios；
- no production Secret/real user Conversation in fixture/log/report。

## 4. Canonical fixtures / evidence

`platform-core/api/fixtures/` is cross-component wire authority for executable samples。

At least：

```text
problem/
identity/
managed-state/
draft/
snapshot/
runtime-control/
gateway-control/
gateway-catalog/
usage/
```

Snapshot fixtures must preserve the S0.1 profile and cover later versioned extensions separately：

```text
OPENAI_CHAT_COMPLETIONS
OPENAI_AUDIO_SPEECH + voice
OPENAI_AUDIO_TRANSCRIPTIONS + optional language
MCP_STREAMABLE_HTTP + authOwnership
Policy
Snapshot v4: Assistant / Memory Seed / Starter
Snapshot v5: ToolGatewayDefinition + surfaceVersion + surfaceHash + clientEnablementPolicy
```

Every T3/T4 report records versions/SHAs/stable states, not credentials or full sensitive payloads。

## 5. System topology

Final system environment exposes one real `platformUrl`：

```text
/.well-known/measix
/api/client/v1/*
/api/admin/v1/*
/admin/*
/runtime/v1/*
```

Harness internal only：

```text
/internal/v1/*
Hub private listener
Relay private control listener
Gateway private control/runtime listener
Adapter internal endpoint
```

Android/Admin cannot use internal shortcuts。

Real processes/resources：

```text
Control Hub + temp hub.db
Runtime Relay + temp relay-spool.db
Enterprise Tool Gateway + applied control/catalog state
Admin production dist/spa
single-origin ingress/router
Android emulator/device
Deterministic Test Adapter
```

## 6. Deterministic Adapter requirements

Required：

```text
OpenAI-compatible Chat request/response
Chat streaming/SSE
OpenAI-compatible TTS binary MP3
OpenAI-compatible ASR multipart transcription
MCP Streamable HTTP
4xx/5xx/timeout
cancellation observation
received header/body capture
request correlation
```

It is test-only, never a production Adapter。

## 7. Android lane

Real emulator/device + test/release-like build：

- starts in Personal Realm, proves Managed content absent, then performs Enrollment；
- real DataStore/Keystore behavior；
- pinned frozen Client fixtures；
- no pre-injected session/snapshot；
- uses client-facing platformUrl；
- no JVM HTTP client replacing Android T4。

## 8. Admin lane

Real browser against production `dist/spa` + real Hub API。

Must create S0.1 runtime resources、S0.2 Assistant/Seed/Starter and Enterprise Update、S0.3 Tool Integration/Candidate Review/Gateway Guidance through public Admin UI only；no manual JSON/DB/internal API。

## 9. Final Golden Scenario — Authoring

`SYS-E2E-001`：

```text
Browser login
→ Create User
→ Generate Enrollment
→ Create Secret
→ Create full Upstream config
→ Test
→ Apply and wait COMPLETED
→ Create Provider + Model
→ Create TTS with voice
→ Create HTTP ASR
→ Create MCP Streamable HTTP
→ Create Managed Assistant + non-empty Memory Seed
→ Create Assistant Starter
→ Create Gateway Tool Integration
→ Test + Refresh Candidate Catalog
→ Review/qualify READ_ONLY tools and edit agent-facing metadata
→ Configure Gateway guidance + highlighted tools
→ Configure Policy
→ Configure Pricing
→ Validate
→ Review + canonical Client Snapshot v5/Gateway surface Preview
→ Publish
→ wait Activation COMPLETED
→ Create + publish Enterprise Update through its independent Feed workflow
```

Assertions：

- candidate != active until Apply；
- no secret in browser persisted state；
- preview contains only client-visible fields；
- preview contains valid Assistant/Seed/Starter references and never embeds Enterprise Update content；
- Snapshot does not duplicate Gateway tool definitions and its `surfaceHash` matches Gateway canonical `tools/list` bytes；
- all Direct runtime resources have valid bindings；
- target including Gateway changes active generation only after Gateway ACK + Relay ACK + Hub finalize; removal follows Control Protocol §6 restrictive path；
- publishing Enterprise Update changes Feed revision/ETag without changing managed generation。

## 10. Final Golden Scenario — Android Consumption

`SYS-E2E-002`：

```text
Android starts in Personal Realm; Managed content absent
→ enter Platform URL + Enrollment Code
→ Enrollment success
→ enter Enterprise Realm
→ bootstrap/preflight
→ fetch/validate/atomic commit Snapshot
→ Managed resources appear with Managed lock in existing selectors
→ Direct Managed MCP enters Enterprise resource availability; only Assistant-bound tools enter that Assistant's model context
→ Gateway initialize + tools/list returns exactly discover_tools, invoke_tool and matches surfaceHash
→ Managed Assistant shows read-only Memory Seed and ordered Starters
→ click “最近企业动态”; prompt is prefilled but not sent
→ user sends; assistant discovers and invokes get_enterprise_updates with no dates and receives latest limit items
→ assistant discovers and invokes one reviewed downstream MCP tool
→ open Enterprise Portal; status and the same Enterprise Update list/detail are visible
→ switch back to Personal Realm
→ Managed resources/tools/assistant/starter/portal entries disappear from picker/registry/resolver
```

Assertions：

- Local records preserved；
- 用户定义单份复用，Personal/Enterprise 运行数据按域与主体隔离；
- `BOUND` does not force the user to remain in Enterprise Realm；
- realm switch does not mutate an already captured interaction context；
- Memory Seed remains separate from mutable Enterprise Local Assistant Memory；
- Portal and Gateway platform tool project one Enterprise Update authority；
- no Enterprise API key/Upstream URL on Android；
- applied generation/hash/release consistent with published Snapshot。

## 11. Required Runtime E2E

### Model

`SYS-RT-001` Managed Chat Completions streaming：

```text
Android existing chat UI
→ Guard/context
→ Relay resource URL
→ deterministic Adapter
→ streaming output
```

Assert model selector, TOOL/REASONING capability metadata, streaming flush, request correlation。

### TTS

`SYS-RT-002` Managed TTS：

- Snapshot model + **voice** used；
- MP3 binary integrity；
- existing playback path works；
- no local voice fallback；
- cancellation propagates。

### ASR

`SYS-RT-003` Managed HTTP transcription：

- Android records/captures audio；
- multipart file/model/language reaches Adapter；
- JSON transcript reaches existing UI；
- no runtime WebSocket；
- Local realtime ASR regression remains Green。

### Direct MCP

`SYS-RT-004` Managed MCP Streamable HTTP：

- existing Android MCP stack；
- `ENTERPRISE_MANAGED|NONE` semantics；
- no upstream secret on Android；
- initialize/list/call minimal flow；
- generation/context gate applies。

### Enterprise Tool Gateway

`SYS-RT-005` Gateway standard MCP surface and governed invocation：

- `initialize` / `tools/list` returns exactly ordered `discover_tools`, `invoke_tool`；
- canonical surface bytes match Snapshot v5 `surfaceHash`；
- discover returns only reviewed, enabled, authorized Published Catalog tools and bounded full schemas；
- invoke accepts only current interaction-bound `toolRef` and schema-valid arguments；
- platform `get_enterprise_updates` and one downstream MCP tool complete through Relay→Gateway；
- Gateway-owned credential is absent from Android, Relay public request and Usage payload；
- Direct/Gateway duplicate source exposure is rejected before Publish。

## 12. Usage / Pricing / Correlation E2E

`SYS-USG-001` after five runtime paths：Browser Usage must show same user/device/generation and the applicable resource kinds：

```text
MODEL
TTS
ASR
MCP
ENTERPRISE_TOOL_GATEWAY
```

Request detail：requestId/interactionId、已解析 resource/typed target、status/duration/forwarded + semantic/cost completeness。Gateway 无普通 Upstream；可信身份已建立但 route 未解析的拒绝请求允许缺 target，按 Control Protocol §18 条件字段记录，不伪造 ID。

`SYS-USG-002` reliable semantic usage + pricing → KNOWN cost where supported。

`SYS-USG-003` missing/partial semantic usage → UNKNOWN/PARTIAL, no fake token/audio/cost。

`SYS-USG-004` filters Time/User/Resource/Kind/Upstream/Status/Completeness work against actual generated traffic。

## 13. Generation update / 428

`SYS-GEN-001`：publish N+1 after Android uses N。

Assertions：

```text
old request
→ Relay 428 managed_snapshot_required
→ forwarded=false
→ Adapter receives no body
→ current Android interaction interrupted
→ Snapshot N+1 sync
→ no automatic replay
→ NEW interaction uses N+1 and succeeds
```

`SYS-GEN-002` publish while stream in flight：request already accepted under captured old state may finish；subsequent request enforces current generation。

## 14. Identity / security scenarios

- `SYS-SEC-001` expired/invalid JWT, unknown kid, wrong audience/issuer；
- `SYS-SEC-002` User Disable；
- `SYS-SEC-003` Device Revoke；
- `SYS-SEC-004` Session revoke/expiry；
- `SYS-SEC-005` restrictive Hub commit persists while Relay unavailable；
- `SYS-SEC-006` permissive Enable only final after Relay ACK；
- `SYS-SEC-007` Admin CSRF missing/wrong；
- `SYS-SEC-008` client Authorization/X-Measix spoof cannot override Relay/upstream policy；
- `SYS-SEC-009` path traversal/absolute URI/host override denied；
- `SYS-SEC-010` management endpoint cannot route through Resource；
- `SYS-SEC-011` Secret absent from Snapshot/Admin persisted DOM-state/Android/log/Usage/report；
- `SYS-SEC-012` deny/428 occurs before Adapter body receive；
- `SYS-SEC-013` Enterprise Session 按 frozen Protocol 由 Enrollment 初始化、成功 authenticated Refresh 轮换凭据并滚动七天 idle expiry；普通请求不续期，闲置超时、Device revoke 和 Disconnect 均安全失效，Access Token lifetime 不被扩张成七天。

## 15. Publish / control convergence

- `SYS-CTL-001` Relay ACK before Admin Publish success；
- `SYS-CTL-001A` target including Gateway requires Gateway ACK before Relay ACK/finalize, even for unchanged catalog bytes; removing Gateway first closes the Relay route and does not block on unreachable Gateway cleanup；
- `SYS-CTL-002` same revision/hash replay idempotent；
- `SYS-CTL-003` same revision/different hash conflicts；
- `SYS-CTL-004` Hub timeout after Gateway/Relay apply reconciles/finalizes same generation；
- `SYS-CTL-005` Hub crash before/after Gateway/Relay ACK recovers；
- `SYS-CTL-006` Gateway or Relay restart fail-closed then Hub rehydrate；
- `SYS-CTL-007` concurrent cross-Relay Admin command cannot overwrite current activation；
- `SYS-CTL-008` historical Republish creates new release/generation；
- `SYS-CTL-009` Upstream SecretVersion Apply changes controlRevision but not managedGeneration unless client capability changed。

## 16. Snapshot / local-first scenarios

- `SYS-SNP-001` Save Draft no Publish → active Client/Relay unchanged；
- `SYS-SNP-002` invalid reference/route/Secret blocks Validate/Publish；
- `SYS-SNP-003` candidate stable IDs preserved；
- `SYS-SNP-004` Snapshot no binding/upstream/internal route/Secret；
- `SYS-SNP-005` corrupt Snapshot preserves Android LKG；
- `SYS-SNP-006` TTS voice required；
- `SYS-SNP-007` HTTP ASR + MCP auth ownership decode correctly；
- `SYS-SNP-008` User/Managed same-ID 分来源引用，更新/恢复不覆盖用户原定义；
- `SYS-SNP-009` Managed mutation rejected at persistence boundary；
- `SYS-SNP-010` Disconnect restores Personal state；
- `SYS-SNP-011` Personal Realm has no Managed picker/tool registry/resolver/execution entry；
- `SYS-SNP-012` 五项策略控制用户原配置及凭据的企业域准入，禁止/收紧/恢复不改写用户配置，运行数据带域与主体归属；
- `SYS-SNP-013` Realm switch preserves Conversation/Assistant/Memory/Attachment/Workspace ownership and active interaction snapshot。
- `SYS-SNP-014` 当前唯一 v4 Assistant/Memory Seed/Starter 与五项用户配置策略，以及 S0.3/S0.4 v5 Gateway resource/surfaceHash projection/reference/hash 同时正确；当前版本准入以 Control Protocol §10.10.1 为权威；
- `SYS-SNP-015` Managed Memory Seed read-only 且不覆盖 Enterprise Local Assistant Memory；
- `SYS-SNP-016` Assistant Starter 只预填 Draft、不 auto-send；
- `SYS-SNP-017` Personal Realm 不展示/注册 Managed Assistant、Starter、Direct Managed MCP 或 Gateway resource。
- `SYS-SNP-018` Snapshot v5 只含 Gateway logical resource/surface metadata/client enablement policy，不复制两个完整 Tool Definition。
- `SYS-SNP-019` Gateway published 后两工具以原子 pair 默认开启；REQUIRED 锁定开启，USER_CONTROLLABLE_DEFAULT_ON 允许按 Deployment/Gateway 保存的 Enterprise-local preference 成对关闭/恢复，Direct Managed MCP 仍保持 Assistant-bound。

## 17. Enterprise Update / Portal scenarios

- `SYS-ERX-001` `get_enterprise_updates` 不传两个日期时按默认/显式 limit 返回最新内容；
- `SYS-ERX-002` start/end 单独或组合过滤、timezone、降序、invalid range 和 truncation 正确；
- `SYS-ERX-003` Portal 与 Gateway platform tool 读取同一 PUBLISHED authority，Draft/Withdrawn 不泄漏；
- `SYS-ERX-004` 更新 Enterprise Update Feed 不改变 `managedGeneration`；
- `SYS-ERX-005` Portal Native Host 状态、同步/刷新/断开操作与 WebView 列表/详情工作，bridge/session/origin fail closed；
- `SYS-ERX-006` Enrollment 与成功 Refresh 按服务端时钟滚动七天 idle expiry；普通 API/Runtime/Portal load 不续期，后台 heartbeat 不保活。

## 18. Usage durability / recovery

- `SYS-MTR-001` every forwarded Runtime request gets req_* and Hub request record；
- `SYS-MTR-002` Hub usage outage → Relay spool persists；
- `SYS-MTR-003` recovery replay；
- `SYS-MTR-004` duplicate request delivery deduped；
- `SYS-MTR-005` poison row diagnosed/retained without blocking all good rows；
- `SYS-MTR-006` full Hub+Relay+Gateway restart keeps backlog/replay and fails closed before rehydrate；
- `SYS-MTR-007` no bytes→token/audio semantic guessing。

## 19. Admin recovery scenarios

- `SYS-ADM-001` browser refresh during Publish recovers activation；
- `SYS-ADM-002` refresh during Upstream Apply recovers same command；
- `SYS-ADM-003` stale Draft 409 preserves local edits；
- `SYS-ADM-004` stale Pricing revision handled；
- `SYS-ADM-005` Secret replace input not recoverable/readable from browser storage；
- `SYS-ADM-006` cursor pagination/load-more real；
- `SYS-ADM-007` Relay/system degraded clearly visible。

## 20. Process / storage recovery

- `SYS-OPS-001` packaged Hub/Gateway/Relay 由三个独立 production services 和 aggregate target 管理，不经过 `go run`/dev server/test harness；
- `SYS-OPS-002` process active 与 application ready 可区分，单 daemon crash 只触发自身有界 on-failure restart/rate-limit，永久配置错误不形成无限 crash loop；
- `SYS-OPS-003` aggregate stop/restart 走 SIGTERM + bounded drain，超时后才 force kill；Gateway/Relay 重启在 Hub rehydrate 前 fail closed；
- `SYS-OPS-004` supervisor collector 可按 service/build/event/correlation 安全收集、轮转、保留 JSON logs，normal/failure evidence 不含 architecture 禁止字段；
- clean DB migration replay；
- upgrade migration from supported dev/RC baseline；
- Hub backup/restore；
- Relay spool recovery；
- Gateway applied control/catalog rehydrate；
- Android process restart with Binding/LKG；
- Keystore roundtrip/invalidation；
- SQLite busy/transient disk failure around critical boundaries；
- no half-release/half-snapshot/half-usage corruption。

## 21. Concurrency / cancellation

- simultaneous Managed interactions share single-flight preflight/sync but distinct context；
- access refresh single-flight；
- publish/control apply with active streams；
- cancel storm releases resources；
- TTS/ASR/Direct MCP cancellation reaches Relay/Adapter；
- Gateway downstream cancellation reaches Gateway/MCP endpoint；
- revoke while waiting sync；
- browser repeated command does not produce duplicate unsafe actions。

## 22. Real Adapter qualification

Final RC must include qualification evidence for every profile the release claims VERIFIED。

Minimum release claim：

```text
OPENAI_CHAT_COMPLETIONS streaming
OPENAI_AUDIO_SPEECH
OPENAI_AUDIO_TRANSCRIPTIONS
MCP_STREAMABLE_HTTP
```

These may be provided by more than one qualified endpoint/Adapter. Architecture does not require one vendor to cover all four。

Qualification records actual correlation/usage capability level；LEVEL_0 is acceptable where semantic usage is unavailable, provided UI correctly shows UNKNOWN/PARTIAL and does not claim precise cost。

## 23. Target VM resource baseline

Record：

```text
Hub idle RSS/CPU
Relay idle RSS/CPU
Gateway idle RSS/CPU
Admin CRUD/Publish latency
Relay first-byte overhead
concurrent stream memory growth
TTS binary buffering behavior
ASR multipart memory/temp-disk behavior
cancel cleanup time
usage backlog drain
50+ tool evaluation search latency/memory
SQLite growth
```

Do not respond to performance failures by default adding Redis/PostgreSQL/message queue；first locate buffering/goroutine/connection/SQLite/logging issues。

## 24. CI / integration gates

### platform-core PR

T0/T1/T2 + affected T3 + S0.1 deterministic smoke。

### S0.1 merge candidate

Full `CAP-*` Gate + freeze manifest。

### S0.2 merge candidate

Full `ERX-*` Gate + current v4 fixtures + pinned real Android/Portal product lane (including required phone scenarios) + freeze manifest。

### S0.3 merge candidate

Full `ETG-*` Gate + current v4 resource checks and target v5 fixtures + 50+ tool evaluation catalog + Gateway/Admin/Test Client lane + production supervision/log collection evidence + freeze manifest。

### Android PR

JVM/concurrency/fixture/HTTP/component + required instrumentation based on change。

### S0.4 merge candidate

Pinned cross-repo emulator lane。

### Final RC

Full scenarios in this document + real Adapter qualification + target VM + backup/restart/replay。

## 25. Final manifest

At least：

```text
architectureCommit
s01FreezeManifestHash
s02FreezeManifestHash
s03FreezeManifestHash
platformCoreCommit
androidCommit
portalCommit
adminBuildHash
portalBuildIdentity
hubBuildIdentity
relayBuildIdentity
gatewayBuildIdentity
clientControlOpenApiHash
gatewayControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
gatewayEvaluationCatalogHash
adapterQualificationRefs
androidAppVersion
scenarioResults
resourceBaselineRef
startedAt
completedAt
```

每个 build/qualification/evidence reference 都必须可定位到内容 hash、exact source/config/profile 与执行结果。不同 profile 的 VERIFIED 不可由一个顶层状态推断；fresh runtime replay 不等于 clean source/build replay。Final harness 负责可执行 schema/验证，不可只检查 scenarios 的 PASS 字符串。No credential/prompt/body in manifest。

## 26. S0 Exit Gate

S0 may be declared complete only when：

1. S0.1 Freeze Gate Green and frozen manifest pinned；
2. S0.2 Enterprise Realm/Experience A/B/C/Portal Gate Green and frozen manifest pinned；
3. S0.3 Enterprise Tool Gateway/Admin/Test Client Gate Green and frozen manifest pinned；
4. Android component/S0.4 Gate Green；
5. all applicable final `SYS-*` critical scenarios Green；
6. OpenAPI/fixtures/codegen no drift；
7. no critical scenario skipped/quarantined；
8. Admin browser clean-environment authoring path Green；
9. Android emulator/device required Model/TTS/ASR/Direct MCP/Gateway path Green；
10. generation 428/no-forward/no-replay Green；
11. Secret no-leak/security scenarios Green；
12. Hub/Relay/Gateway crash/reconcile + usage spool/replay Green；
13. real Adapter qualification supports release claim；
14. Personal/Enterprise ClientRealm isolation and Local Android runtime regression Green；
15. backup/restore/process restart 与 production supervision/graceful lifecycle/log collection-redaction Green；
16. target VM resource baseline acceptable or explicit accepted deviation recorded；
17. final manifest pins architecture/platform-core/android/portal/Gateway inputs and is reproducible。

Only then may roadmap progress to S1 Agent Space without carrying an unresolved S0 client/server contract debt。

## 27. 不属于本文

S1 Agent Space/Agent Runtime、Phase 2 multi-tenancy/RBAC/rollout、Provider private protocol correctness、production observability/SLO platform are outside S0 final test authority。
