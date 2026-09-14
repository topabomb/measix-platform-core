# S0.1 — Managed Capability Delivery 实施决议

> 状态：S0.1 Implementation Decision / 实施顺序权威  
> 版本：2026-08-31
> 上位文档：`measix-s0-capability-delivery-contract-spec.md`  
> 全局技术决策：`measix-s0-implementation-decision.md`  
> Wire 权威：`measix-s0-control-protocol.md`  
> 测试权威：`measix-s0-capability-delivery-system-testing-spec.md`  
> 文档职责：定义 S0.1 在 `measix-platform-core` 中的实施顺序、TDD checkpoint 和 Freeze handoff；不维护 concrete source tree、Vue/Go file names 或 executable schema。

## 1. 执行原则

S0.1 固定方向：

```text
Android reality audit (read-only)
→ Architecture contract closure
→ executable contract Red tests
→ server/Admin implementation
→ component/cross-component Green
→ real Browser + Test Client product proof
→ real Adapter qualification/resource evidence
→ Client Contract Freeze
```

不能采用：

```text
先写 Android
→ 发现 Snapshot 不够
→ 临时改 server DTO
→ 再补 Admin/contract
```

S0.1 的实现仓库是 `topabomb/measix-platform-core`；`rikkahub_mcp` 仅作为 consumer reality 输入，不落 Enterprise integration。

## 2. TDD 纪律

每个行为变化：

```text
architecture / executable contract when applicable
→ failing behavioral test (Red)
→ minimal implementation
→ Green
→ refactor
→ affected full gate
```

Red 应证明真实缺口，例如 schema 接受错误 vocabulary、required resource 缺字段、UI 无法产生 valid payload、enabled resource 缺 binding、Preview 与 canonical compiler 不一致、deny path 仍 forward、Usage completeness 被伪造等。

禁止通过降低断言、隐藏 UI、长期 skip、固定大 sleep 或 retry-until-green 获得 Green。

具体 test files/commands 由 `measix-platform-core/docs/testing.md` 维护。

## 3. C0–C7 checkpoint

```text
C0 Contract Audit & Freeze Preparation
  ↓
C1 Upstream Operational Completion
  ↓
C2 Managed Resource Editor Completion
  ↓
C3 Snapshot Projection & Preview
  ↓
C4 Runtime Reference Profile Completion
  ↓
C5 Usage / Pricing / Observability Completion
  ↓
C6 System Harness & Browser/Test Client E2E
  ↓
C7 S0.1 Freeze Gate
```

后续 checkpoint 的局部代码可以提前并行，但 completion claim 必须遵守前置 contract/evidence dependency。

## 4. C0 — Contract Audit & Freeze Preparation

### 目标

建立唯一映射：

```text
Architecture Definition
↔ Control Protocol
↔ executable Admin/Client/Internal OpenAPI
↔ canonical fixtures
↔ generated Go/TS types
↔ backend domain
↔ Android consumer reality (read-only)
```

### 必须收敛

至少包括：

- clientProtocol typed/enforced vocabulary；
- Model modalities/capabilities；
- TTS voice；
- HTTP ASR semantics + optional language；
- MCP auth ownership；
- Policy defaults/references；
- client-visible Definition vs server-only Binding；
- unknown optional response / unknown request compatibility；
- deterministic codegen/fixtures。

C0–C7 使用当前唯一协议与实际 schemaVersion；未发布原型无兼容义务，修改后必须重新执行受影响 gate。

### Checkpoint

Architecture ↔ protocol ↔ executable schema 无已知冲突，required positive/negative fixtures 和 generated types 可重复，server 只接受合法 required profile，Android source 未修改。

## 5. C1 — Upstream Operational Completion

### 目标

Admin 创建/编辑的 Upstream 必须等价于 backend 可以真实 Test/Apply 的 candidate，而不是占位表单。

### Required product/domain closure

```text
Name
Base URL
Transport Capabilities
Auth: NONE / BEARER / STATIC_HEADER / BASIC
Secret create/select/replace
Correlation Mode
Usage Capability Level
Timeout Defaults
Candidate Revision
Active Revision
Verification/Status
Test Connection
Apply
```

Secret plaintext submit 后清除，不进入 browser persistence；candidate 与 active revision 始终分离。

### Checkpoint

clean environment 只通过 public UI 创建 valid Upstream；candidate edit/secret replace 不隐式改变 active runtime；Test 不 Apply；Apply 通过 authoritative Activation/Relay ACK；refresh/failure 保持正确 candidate/active semantics。

## 6. C2 — Managed Resource Editor Completion

### 目标

Resources 成为完整 Managed Capability authoring surface，而不是 Model-only 或 raw JSON editor。

Required tabs：

```text
Models
TTS
ASR
MCP
Policy
```

所有 editor 使用 typed controls，支持 server-side execution binding 和 validation navigation。

### Model

Provider、Display Name、Upstream Model Key、required profile、TEXT/IMAGE input、TEXT output、TOOL/REASONING、Upstream/runtimePath、Enabled。

### TTS

Display Name、`OPENAI_AUDIO_SPEECH`、Upstream Model Key、Voice(required)、Upstream/runtimePath、Enabled。

### ASR

Display Name、`OPENAI_AUDIO_TRANSCRIPTIONS`、Upstream Model Key、optional Language、Upstream/runtimePath、Enabled；无 Realtime/WebSocket/VAD controls。

### MCP

Display Name、`MCP_STREAMABLE_HTTP`、Auth Ownership `ENTERPRISE_MANAGED|NONE`、Upstream/runtimePath、Enabled。

### Policy

四类 Local coexistence + Default Model/TTS/ASR；default reference 必须 valid/enabled。

### Checkpoint

required resource 全部能通过可视化 typed editor create/edit/delete；binding/reference/validation closure 正确；不要求管理员手写 internal ID、enum、route 或 JSON。

## 7. C3 — Snapshot Projection & Preview

### 目标

Draft 到 Client Snapshot 只有一个 canonical projection authority。

必须实现：

```text
Draft/Policy validation
→ canonical client projection
→ Preview
→ Stage/Publish uses same projection
```

Preview：无 Release/generation/control side effect；不含 Secret/Upstream URL/runtimeRouteId/server-only Binding/Pricing；deterministic order/hash semantic；Admin Review 显示 resource/policy/runtime impact。

### Checkpoint

Preview 与 final published client projection 同源；invalid Draft 不能产生新 active generation；warning review 语义正确。

## 8. C4 — Runtime Reference Profile Completion

### 目标

使用 deterministic Adapter/Test Client 证明 S0.1 required transport/profile，而不是只证明 backend CRUD。

```text
Model  OPENAI_CHAT_COMPLETIONS: request/response + SSE + cancel
TTS    OPENAI_AUDIO_SPEECH: model/input/voice + binary MP3
ASR    OPENAI_AUDIO_TRANSCRIPTIONS: multipart + cancel
MCP    MCP_STREAMABLE_HTTP: initialize/list/call + stream/cancel where applicable
```

Relay 必须保持 transparent/provider-agnostic，且 old generation/auth/resource/path deny 在 forward body 前完成。

### Checkpoint

四类 resource 都从 published Snapshot 获取 identity/runtime metadata，通过 Client-facing topology 执行并形成 request usage；不依赖 internal route/upstream knowledge。

## 9. C5 — Usage / Pricing / Observability Completion

### 目标

管理员能从 runtime request 追到 resource/upstream/generation/status/usage/cost/health，而不是只有日志。

必须闭合：

- Request Usage durable delivery + dedupe；
- MODEL/TTS/ASR/MCP classification；
- reliable semantic meters；
- semantic EXACT/PARTIAL/UNKNOWN 与 cost KNOWN/PARTIAL/UNKNOWN 分开；
- Pricing rule/effective selection；
- filters/detail；
- Overview/System desired-vs-applied、Activation、Relay/Upstream、spool/semantic unknown diagnostics。

### Checkpoint

无 semantic source 时不猜 token/audio/cost；四类 resource 的 usage/product filters 和 diagnostic state 可通过 Admin 产品面观察。

## 10. C6 — Product/System E2E

C6 不是“Go system tests 都 Green”即可完成。它必须同时包含：

```text
production Admin SPA + real browser
real Control Hub
real Runtime Relay
deterministic Adapter
Test Client using public Client Control/Runtime surface
real persistence/process restart boundaries
```

Browser 必须从 clean environment 完整执行 Admin golden path；Test Client 完成四 capability；Usage 回到 Admin 可审查；new generation、refresh、restart、security、backup/recovery 等 required scenarios 按 System Testing Spec 执行。

任何 internal API/manual JSON/DB shortcut 都不能作为 product Golden Path proof。

## 11. C7 — Client Contract Freeze Gate

C7 只在 C6 全部 required scenario Green 后执行。

Freeze evidence 至少 pin：

```text
architecture commit
platform-core candidate commit
Admin production build identity
Client Control OpenAPI identity
canonical fixture identity
Snapshot schema version
Deterministic Adapter identity
real Adapter qualification evidence
required scenario results
resource/performance baseline evidence
clean replay result
```

Manifest 是 exact candidate evidence，不是手写 planning/status 文档。candidate SHA 或相关 architecture/client contract 变化后必须重新运行受影响 gate。

## 12. S0.1 Exit / S0.2 Handoff

S0.1 Exit 只有两个输出：

1. **产品闭环已被 executable evidence 证明**；
2. **Android-visible Client Control/Snapshot contract 已由 exact Freeze 固定**。

然后 S0.2 才允许实现 Enterprise Realm、A/B/C 最小投影和 Portal MVP。S0.2 当前 Snapshot v4 包含 Assistant/Memory Seed/Starter，不保留旧原型兼容；S0.3 先冻结 Enterprise Tool Gateway 与 Snapshot v5，S0.4 才完成 Android full-profile integration。任何阶段发现 frozen contract 不足时，都按 compatibility/versioning 规则回到 architecture/protocol 层处理，不能在 Android 本地发明隐式字段或 fallback。
