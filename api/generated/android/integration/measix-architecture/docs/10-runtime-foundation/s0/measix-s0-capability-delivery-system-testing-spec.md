# S0.1 — Managed Capability Delivery System Testing Spec

> 状态：S0.1 System Testing Authority / pre-Android Gate  
> 版本：2026-08-29
> 上位文档：`measix-s0-capability-delivery-contract-spec.md`  
> 实施决议：`measix-s0-capability-delivery-implementation-decision.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> 最终 S0 测试：`measix-s0-system-testing-spec.md`  
> 文档职责：定义 S0.1 在 Android integration 之前必须通过的 contract/component/cross-component/browser/Test Client system scenarios 和 Freeze evidence；不重新定义产品或 wire。

## 1. Gate 目标

S0.1 测试必须证明：

```text
Admin Console
  → Control Hub
  → Runtime Relay
  → Deterministic / qualified Adapter
  → Usage / Pricing

Test Client
  → Client Control Snapshot
  → Runtime Relay
  → Adapter
```

这不是最终 Android T4。S0.1 的 Test Client 只验证“server-side client-facing contract 已经可消费”；S0.2 验证 A/B/C/Portal 最小产品闭环，S0.3 验证 Gateway/Admin/Test Client 服务端闭环，S0.4/最终 S0 才用真实 Android emulator/device 验证完整 Android runtime boundary。

## 2. 测试分层

| 层级 | 名称 | S0.1 目的 |
|---|---|---|
| T0 | Static / Contract | Architecture→OpenAPI→fixture→generated types 一致，required profile typed/frozen-ready |
| T1 | Unit / Domain | resource validation、projection、pricing、classification、URL/path/security pure rules |
| T2 | Component Integration | Hub/Relay/Admin 单组件 + SQLite/HTTP/DOM 真实边界 |
| T3 | Cross-component Integration | real Hub↔Relay、Admin↔Hub、Relay↔deterministic Adapter |
| T4.1 | S0.1 Product/System | real browser + Hub + Relay + Adapter + Test Client + Usage，pre-Android complete loop |

`T4.1` 不替代最终 `T4` Android System Gate。

## 3. 确定性与真实边界

每个测试：

- 独立 temp directory/SQLite/ports/identity；
- 默认不访问公网；
- 真实 SQLite + migrations；
- 真实 TCP/HTTP streaming/binary/multipart；
- Hub/Relay T3/T4.1 使用真实 process/binary；
- Admin T4.1 使用 production `dist/spa`；
- async convergence 使用 bounded polling，不使用固定大 `sleep`；
- critical scenario 不自动 retry 成 Green；
- Secret/credential/prompt/body 不进入 test report；
- deterministic Adapter 可记录是否收到 request/body，用于 admission proof。

## 4. Canonical Fixtures

`platform-core/api/fixtures/` 至少覆盖：

```text
managed-state/
draft/
snapshot/
runtime-control/
usage/
problem/
```

S0.1 required Snapshot fixtures：

```text
snapshot/model-openai-chat.json
snapshot/tts-openai-speech.json
snapshot/asr-openai-transcription.json
snapshot/mcp-streamable-http.json
snapshot/full-required-profile.json
snapshot/unknown-optional-field.json
snapshot/invalid-secret-leak.json       # negative generator/validation case
snapshot/unsupported-protocol.json      # negative
```

实际命名可适配现有 fixture tree，但语义覆盖必须存在。

## 5. Critical Scenario ID

S0.1 使用 `CAP-*`：

```text
CAP-C0-*  contract/freeze preparation
CAP-C1-*  upstream operational
CAP-C2-*  resource authoring
CAP-C3-*  snapshot/review
CAP-C4-*  runtime transports
CAP-C5-*  usage/pricing/observability
CAP-C6-*  product/system
CAP-C7-*  freeze/handoff
```

这些 ID 只用于 S0.1 长期 Gate；普通 unit test 不编号。

## 6. C0 — Contract Scenarios

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C0-001 | Required profile enum/schema | Model/TTS/ASR/MCP required protocol 能确定性 encode/decode |
| CAP-C0-002 | Unsupported normal product protocol | Admin/Validate 不把 native Anthropic/Google/Realtime 当 S0.1 supported capability |
| CAP-C0-003 | Model vocabulary | modalities/capabilities 只接受 frozen vocabulary，未知 request value 被拒绝 |
| CAP-C0-004 | TTS voice | enabled Managed TTS 无 voice 时 validation fail |
| CAP-C0-005 | ASR profile | HTTP 文件转写、OpenAI/DashScope 实时识别可表达；按协议验证参数、传输及共享快照，禁止跨类型字段 |
| CAP-C0-006 | MCP auth | `ENTERPRISE_MANAGED|NONE` valid；未定义 user-managed relay 不被接受为 S0.1 profile |
| CAP-C0-007 | unknown optional response | generated/consumer test ignores unknown optional response field |
| CAP-C0-008 | unknown request field | request contract rejects unknown field according to global rule |
| CAP-C0-009 | codegen drift | Go/TS generated artifacts reproducible |

## 7. C1 — Upstream Operational Scenarios

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C1-001 | Browser creates NONE-auth Upstream | backend accepts exact UI payload; candidate rev=1; active absent |
| CAP-C1-002 | BEARER secret | browser never reads plaintext back; SecretRef valid |
| CAP-C1-003 | STATIC_HEADER/BASIC | header/user/password SecretRef validation correct |
| CAP-C1-004 | Replace Secret | new immutable version created; active runtime unchanged before Apply |
| CAP-C1-005 | Candidate edit | configRevision grows; activeConfigRevision unchanged |
| CAP-C1-006 | Test connection | reachable/latency/verified capability/warnings display correctly |
| CAP-C1-007 | Apply happy path | 202 activation → Relay ACK → activeConfigRevision changes |
| CAP-C1-008 | Apply refresh recovery | browser reload continues same activation instead of starting second command |
| CAP-C1-009 | Apply failure | old active runtime remains; candidate edits remain visible |
| CAP-C1-010 | Secret browser safety | input cleared after submit; no plaintext localStorage/Pinia/DOM after close |

## 8. C2 — Resource Authoring Scenarios

### Models

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C2-001 | Create Provider + Model | valid prv_*/mdl_* retained across save/reload |
| CAP-C2-002 | Model capabilities | TEXT/IMAGE + TOOL/REASONING persist and validate |
| CAP-C2-003 | Missing Provider | validation error; publish blocked |
| CAP-C2-004 | Missing Binding | enabled model cannot publish |

### TTS

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C2-010 | Create TTS | model key + voice + binding persist |
| CAP-C2-011 | Missing voice | validation error |

### ASR

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C2-020 | Create HTTP ASR | model key/language/path/binding persist |
| CAP-C2-021 | Realtime fields | not exposed as Managed S0.1 editor/config |

### MCP

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C2-030 | Enterprise-managed MCP | Streamable HTTP + server-side auth binding valid |
| CAP-C2-031 | NONE auth MCP | no unnecessary Secret required |
| CAP-C2-032 | unsupported auth ownership | validation blocked |

### Policy / Draft

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C2-040 | 当前 policy 校验 | 五项必填布尔授权完整；拒绝旧版本和缺失策略，详见 S0.2 Testing |
| CAP-C2-041 | Defaults | default IDs must exist and be enabled |
| CAP-C2-042 | stale revision | 409 preserves local edits; no silent overwrite |
| CAP-C2-043 | route safety | invalid path/method/transport rejected |

## 9. C3 — Snapshot / Review Scenarios

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C3-001 | Preview canonical projection | Preview and StageRelease use same projection logic |
| CAP-C3-002 | Preview has no side effect | no Release/generation/controlRevision change |
| CAP-C3-003 | Snapshot no server internals | no upstreamId/baseUrl/runtimeRouteId/Secret/plain credential |
| CAP-C3-004 | Full profile Snapshot | Provider/Model/TTS/ASR/MCP/Policy all present with frozen fields |
| CAP-C3-005 | deterministic order/hash | same semantic Draft → same canonical projection/hash where metadata fixed |
| CAP-C3-006 | ETag/hash | download ETag and body snapshotHash consistent |
| CAP-C3-007 | Publish diff | Added/Changed/Removed + policy/runtime impact visible |
| CAP-C3-008 | Warning acknowledgement | publish requires explicit acknowledgement where contract says warning |
| CAP-C3-009 | invalid Draft Publish | no new active generation; previous active unchanged |

## 10. C4 — Runtime Transport Scenarios

### Model

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C4-001 | Chat request/response | payload transparently reaches Adapter and response returns |
| CAP-C4-002 | Chat streaming | first chunk/continued chunks pass without full buffering |
| CAP-C4-003 | Model cancel | client cancel observed by Relay/Adapter |

### TTS

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C4-010 | TTS request | `model/input/voice` semantics reach test Adapter |
| CAP-C4-011 | Binary integrity | returned MP3 test bytes identical; no JSON buffering/corruption |
| CAP-C4-012 | TTS failure | upstream status/timeout follows Relay error contract |

### ASR

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C4-020 | Multipart transcription | file/model/language fields preserved |
| CAP-C4-021 | Multipart limits | oversized/invalid request handled at correct boundary |
| CAP-C4-022 | ASR cancel | upload/request cancellation releases resources |

### MCP

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C4-030 | MCP initialize/list/call minimal flow | Streamable HTTP path/body/content type preserved |
| CAP-C4-031 | MCP streaming/session headers where applicable | allowed headers preserved without enterprise credential leak |
| CAP-C4-032 | MCP auth injection | enterprise-managed upstream credential only injected server-side |

### Admission

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C4-040 | old generation | 428 + forwarded=false + Adapter received no body |
| CAP-C4-041 | invalid JWT | 401 + no forward |
| CAP-C4-042 | revoked principal | 403 + no forward |
| CAP-C4-043 | unknown resource | deny + no forward |
| CAP-C4-044 | path traversal/absolute URI | deny + no forward |
| CAP-C4-045 | internal header spoof | client cannot override internal/upstream auth/correlation policy |

## 11. C5 — Usage / Pricing Scenarios

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C5-001 | Model request usage | request fact persisted and classifies MODEL |
| CAP-C5-002 | TTS request usage | classifies TTS; semantic characters/audio accepted when reliable |
| CAP-C5-003 | ASR request usage | classifies ASR; semantic audio seconds accepted when reliable |
| CAP-C5-004 | MCP request usage | classifies MCP; no token guessing |
| CAP-C5-005 | Hub usage outage | Relay spool persists/replays; no silent loss |
| CAP-C5-006 | duplicate delivery | requestId dedupe prevents double count |
| CAP-C5-007 | poison row | poison retained/diagnosed without blocking all good rows |
| CAP-C5-008 | UNKNOWN semantic | no false token/audio/cost value |
| CAP-C5-009 | PARTIAL semantic | partial dimensions/cost clearly marked |
| CAP-C5-010 | Pricing known | reliable meter + valid rule → KNOWN cost |
| CAP-C5-011 | Pricing missing | cost UNKNOWN/PARTIAL per available facts |
| CAP-C5-012 | effective pricing | effectiveFrom/revision semantics correct |
| CAP-C5-013 | filter time/user/resource | only matching requests returned |
| CAP-C5-014 | filter kind/upstream/status/completeness | all S0.1 product filters work |
| CAP-C5-015 | request detail safety | no prompt/body/Secret |

## 12. C5 — Overview/System Diagnostics Scenarios

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C5-020 | Hub/Relay converged | desired/applied revision+hash visible |
| CAP-C5-021 | Relay out of sync | high-priority degraded/warning state visible |
| CAP-C5-022 | Activation applying/failed | current/last activation diagnostic visible |
| CAP-C5-023 | Upstream degraded | status visible without pretending all resources healthy |
| CAP-C5-024 | Usage lag/backlog | ingest/spool diagnostic visible |
| CAP-C5-025 | semantic orphan/unknown | observable, not silently dropped |

## 13. C6 — Browser + Test Client Product Scenario

### CAP-C6-001 — Clean environment golden path

Browser performs only public Admin UI actions:

```text
login
create user + enrollment
create secret
create upstream
Test
Apply
create Provider + Model
create TTS
create ASR
create MCP
configure Policy
configure Pricing
Validate
Review Client Snapshot Preview
Publish
wait activation completed
```

Assertions：

- no manual JSON/database/internal endpoint；
- every saved object reloads correctly；
- candidate/active semantics remain visible；
- preview matches final published projection；
- active generation increments only after successful publish。

### CAP-C6-002 — Test Client four-capability path

Test Client uses client-facing topology to obtain published state/snapshot, then invokes：

```text
Model streaming
TTS binary
ASR multipart
MCP Streamable HTTP
```

Assertions：

- all four resource IDs originate from Snapshot；
- no Test Client knowledge of upstreamId/runtimeRouteId/base URL；
- all calls appear in Usage；
- correlation/requestId trace is closed。

### CAP-C6-003 — Usage review

Browser returns to Usage/System and verifies：

- four resource kinds；
- filters；
- details；
- known/partial/unknown semantic/cost；
- runtime/relay health。

### CAP-C6-004 — Publish new generation

Publish changed resource/policy：

- previous generation request gets 428/no-forward；
- Test Client fetches new snapshot；
- new generation succeeds；
- Usage records generation accurately。

## 14. C6 — Failure / Recovery Scenarios

| ID | 场景 | 必须断言 |
|---|---|---|
| CAP-C6-010 | Hub crash around Publish | persisted activation + Relay status reconciles without duplicate generation |
| CAP-C6-011 | Relay restart | fail-closed then Hub rehydrate; runtime returns READY |
| CAP-C6-012 | Browser refresh during activation | same activation recovered |
| CAP-C6-013 | SQLite busy/transient | bounded error/retry semantics, no corruption |
| CAP-C6-014 | full Hub+Relay restart | active release/route/usage spool recover |
| CAP-C6-015 | backup/restore | IDs/release/generation/pricing/usage preserved per S0 baseline |

## 15. Security Scenarios

S0.1 critical security suite 至少：

- invalid/expired/wrong-aud JWT；
- disabled user/revoked device/session；
- Admin CSRF/session failure；
- runtime absolute URI/`..`/encoded traversal/host override；
- internal management endpoint not reachable through Resource route；
- client Authorization/X-Measix headers cannot override Relay policy；
- upstream Set-Cookie/redirect behavior according to Relay contract；
- Secret absent from Snapshot/Admin persisted browser state/log/Usage/report；
- 401/403/428/resource deny before Adapter body receive；
- Snapshot Preview cannot expose server-only fields。

## 16. Real Adapter Qualification Gate

Deterministic tests Green 后，至少记录：

```text
adapterName/version
endpoint/profile
OPENAI_CHAT_COMPLETIONS result
streaming/cancel result
TTS result if claimed
ASR result if claimed
MCP endpoint result
correlation level
usage capability level
known deviations
```

S0.1 不要求所有能力必须来自同一家真实 Adapter；但每个被 release 宣称为 VERIFIED 的 capability/profile 都必须有 qualification evidence。

## 17. Performance / Resource Baseline

S0.1 不是峰值 QPS benchmark，但 target small VM 至少记录：

```text
Hub idle RSS/CPU
Relay idle RSS/CPU
Admin CRUD/Publish baseline latency
Relay first-byte overhead
concurrent streaming memory growth
multipart temporary memory/disk behavior
cancel release time
usage backlog drain
SQLite growth
```

性能问题优先修复 buffering/goroutine/connection/DB/logging，不因为测试压力默认引入 Redis/PostgreSQL。

## 18. Freeze Manifest

CAP-C7-001 必须生成：

```text
architectureCommit
platformCoreCommit
adminBuildHash
clientControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
deterministicAdapterVersion
realAdapterQualificationRef
scenarioResults
startedAt
completedAt
```

CAP-C7-002 必须证明：从同一个 manifest 可以在 clean environment 重跑 required S0.1 system path。

## 19. S0.1 Exit Gate

只有全部满足才可标记 S0.1 完成：

1. 所有 CAP required scenarios Green；
2. 无 critical skip/quarantine；
3. OpenAPI/fixture/codegen 无 drift；
4. Admin production build/browser golden path Green；
5. real Hub/Relay + deterministic Adapter/Test Client four-capability path Green；
6. usage spool/replay/dedupe/unknown semantics Green；
7. security/no-forward/no-secret-leak scenarios Green；
8. at least required real profile qualification evidence present；
9. target resource baseline 无未解释 blocker；
10. Freeze manifest 生成并可重放；
11. 当前 client-visible baseline 及其实际 schemaVersion 固定。

S0.1 Exit 后才能进入 `measix-s0-enterprise-realm-experience-contract-spec.md` 定义的 S0.2；S0.2 Freeze 后先进入 Enterprise Tool Gateway Contract 定义的 S0.3，S0.3 Freeze 后才能进入 Android Integration Contract 定义的 S0.4。

## 20. 与最终 S0 System Testing 的关系

最终 `measix-s0-system-testing-spec.md` 仍是 S0 Exit Authority。S0.1 scenarios 可以被最终 T4 复用，但最终 Gate 必须把 Test Client 替换/补充为真实 Android emulator/device，并固定：

```text
platform-core SHA
rikkahub_mcp SHA
S0.1 freeze manifest
```

不能因为 S0.1、S0.2 或 S0.3 已通过就跳过 Android persistence/Keystore/Effective Runtime/428 interaction 等 S0.4-specific proof。
