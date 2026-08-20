# S0.1 Platform Core 进展

> Architecture baseline: `topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`  
> 当前阶段：**S0.1 Managed Capability Delivery**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

S0 Core 基础已建立：Identity/Enrollment、Draft/Release/Snapshot、Relay desired-state apply/admission/transport、Usage spool/ledger/pricing、Admin shell。

**S0.1 尚未完成。** Client Control/Snapshot v1 仍是 pre-freeze。不能进入 S0.2 Android，不能宣称 S0 Exit。

Hub 侧与 Relay 侧全部 MUST 测试场景已覆盖并验证 Green（见 `docs/s0-review-report.md`）。

## C0–C7 状态

| Checkpoint | 状态 | 当前主要缺口 |
|---|---|---|
| C0 Contract Audit & Freeze Prep | ✅ Green | Provider/Model/TTS/ASR/MCP 枚举闭合完成；Snapshot fixtures 完成；codegen drift Green |
| C1 Upstream Operational | ✅ Green | UpstreamConfig 完整表单（auth/timeout/correlation/usage level）+ component TDD 2 tests Green |
| C2 Managed Resource Editor | ✅ Green | Models/TTS/ASR/MCP/Policy 五类 editor + relationship count + TDD 7 tests Green |
| C3 Snapshot Projection & Preview | ✅ Green | `POST /api/admin/v1/draft:preview` 端点 + PreviewDraftRequest/Response schema + handler + service + UI dialog + TDD 1 test Green |
| C4 Runtime Reference Profile | ⏳ 未完成 | 缺 Test Client/Test Adapter 四 profile (SSE/Binary/Multipart/MCP) 完整闭环 |
| C5 Usage / Pricing / Observability | ⏳ 未完成 | Admin 缺 resource-kind 视角、完整 filters、UNKNOWN/PARTIAL/cost semantics 与趋势可视化 |
| C6 Browser + Hub + Relay System E2E | ⏳ 未完成 | 缺 real browser + real Hub/Relay + deterministic Test Client/Test Adapter 的 S0.1 Gate 证据 |
| C7 Client Contract Freeze Gate | ⏳ 未开始 | C4–C6 未全部 Green；freeze manifest 不存在 |

## 已完成的变更清单

### C0 Contract Audit & Freeze Preparation

1. **OpenAPI 枚举闭合**：
   - `client-control.openapi.yaml`: Provider `clientProtocol` enum → `OPENAI_CHAT_COMPLETIONS`
   - `admin.openapi.yaml`: Model `capabilities`/`inputModalities`/`outputModalities` enum 化
   - `admin.openapi.yaml`: ASR `clientProtocol` enum → `OPENAI_AUDIO_TRANSCRIPTIONS`
   - `admin.openapi.yaml`: ASR 新增 `language` optional 字段
   - `client-control.openapi.yaml`: 对应 Client 侧 schema 同步闭合

2. **后端 validation/snapshot 对齐**：
   - `snapshot.go`: 编译器使用强类型枚举转换（`clientapi.ProviderDefinitionClientProtocol` 等）
   - `service.go`: `validateContent` 使用 `.Valid()` 校验所有枚举字段

3. **Canonical fixtures**：
   - `api/fixtures/snapshot/`: full-required-profile, model-openai-chat, tts-openai-speech, asr-openai-transcription, mcp-streamable-http
   - `api/fixtures/invalid/`: snapshot-invalid-capability, snapshot-invalid-modality, snapshot-unsupported-protocol

4. **Codegen drift**：
   - `admin.gen.go` / `client.gen.go`: 枚举类型和 `Valid()` 方法已生成
   - `console/src/api/generated.ts`: 前端类型已重新生成
   - `api/generated/android/`: Android wire 已同步

### C1 Upstream Operational

1. **UpstreamsPage.vue 重写**：
   - 替换简化 `providerKind` 表单为完整 `UpstreamConfig`
   - Auth section: NONE/BEARER/STATIC_HEADER/BASIC + SecretRef
   - Timeouts: connectMs/responseHeaderMs/idleMs
   - CorrelationMode（typed enum）: HEADER_ECHO/VIRTUAL_KEY/REQUEST_LOG_ID/USAGE_API/WEBHOOK/NONE
   - UsageCapabilityLevel: LEVEL_0/LEVEL_1/LEVEL_2
   - TransportCapabilities（typed enum）: HTTP_REQUEST_RESPONSE/HTTP_STREAMING_SSE/HTTP_BINARY_STREAM/HTTP_MULTIPART

2. **UpstreamsPage.test.ts**（2 tests Green）：
   - 不渲染 Provider kind select
   - 提交包含所有必需字段且不含 `providerKind`

3. **Admin executable contract closure（P1-A，head `4db0d67`）**：
   - `UpstreamConfig.transportCapabilities` items 改为 frozen enum（4 值），`correlationMode` 改为 frozen enum（6 值）
   - `auth` 从 `additionalProperties: true` 松散 map 改为 closed 类型化 `UpstreamAuth`（type/secretRef/headerName/username/passwordSecretRef）
   - 新增 `SecretRef`（secretId+secretVersion）closed schema
   - 重新生成 `admin.gen.go` / `generated.ts`；后端 handler、runtimecontrol、测试全部改用 typed 字段
   - UpstreamsPage 使用正确枚举值，STATIC_HEADER 绑定 headerName、BASIC 绑定 username+passwordSecretRef，Test 结果结构化渲染
   - 新增 5 个契约测试（frozen enum、closed auth、SecretRef、typed ref）全部 Green

4. **Admin Release read model 扩展（P1-B，head `aa08935`）**：
   - `Release` 补全 publish provenance：sourceDraftRevision、publishedAt、publishedBy、diffSummary、activationHistory
   - 新增 `DiffSummary`（added/changed/removed + 按 kind 的 `details`）、`ResourceDiff`、`ActivationSummary` schemas
   - backend `ListReleases`/`GetRelease`/publish 均通过 `buildReleaseView` 计算 diff（对比前一 release 的 draft content，structural JSON hash）与 activation history（按 target_generation 查询）
   - ReleasesPage 显示 diff summary、published by/at、detail 弹窗含 per-kind 表格与 activation timeline
   - 新增 3 个契约测试 + 2 个 `releaseContentDiff` 单元测试，全部 Green

### C2 Managed Resource Editor

1. **ResourcesPage.vue 重写**：
   - 新增 TTS/ASR/MCP editor sections（Add/Delete/Edit）
   - 新增 Policy editor（4 toggle for allowLocalProviders/Tts/Asr/Mcp）
   - 新增 Providers card with relationship count
   - 保留 Models editor 和 Validation panel

2. **draft.ts store 扩展**：
   - `addTts()`, `addAsr()`, `addMcp()` 方法
   - 各方法正确初始化 `clientProtocol` 默认值和 `runtimePath`

3. **ResourcesPage.test.ts**（7 tests Green）：
   - 五类资源 section 渲染
   - TTS/ASR/MCP Add 按钮功能
   - Policy toggle 渲染
   - Provider-Model relationship count
   - Snapshot Preview API 调用和对话框渲染

### C3 Snapshot Projection & Preview

1. **OpenAPI 契约**：
   - `POST /api/admin/v1/draft:preview` 路由
   - `PreviewDraftRequest`: `{ expectedDraftRevision: int }`
   - `DraftPreviewResponse`: `{ draftRevision, snapshotHash, providers, models, tts, asr, mcp, policy }`

2. **后端实现**：
   - `capability.Service.PreviewDraft()`: 编译 draft 为 snapshot 预览，返回 hash 和资源列表
   - `httpapi.PreviewDraft()`: HTTP handler with auth/conflict/error mapping
   - `admin.gen.go`: 自动生成 `PreviewDraft` 接口和路由注册

3. **前端 UI**：
   - ResourcesPage 添加 "Preview" 按钮
   - Snapshot Preview 对话框显示 hash、revision 和资源计数

4. **测试**：
   - 前端 1 test Green（验证 API 调用 + 对话框内容）
   - 后端全部测试 Green（包括新增 handler）

### P0 依赖降级

1. `@quasar/app-vite` 3.x → 2.3.0（兼容 Node 22.17.0）
2. `vue-router` → 4.5.1
3. `pinia` → 3.0.4
4. `console/quasar.config.ts`: `#q-app` → `@quasar/app-vite/wrappers`
5. `console/src/boot/api.ts`: import path 修正
6. `console/src/router/index.ts`: import path 修正

## 测试执行结果

### 后端 Go 测试（全部 Green）

```text
internal/contract        — ok
internal/hub/adminstatic — ok
internal/hub/app         — ok
internal/hub/capability  — ok
internal/hub/httpapi     — ok
internal/hub/identity    — ok
internal/hub/maintenance  — ok
internal/hub/runtimecontrol — ok
internal/hub/security    — ok
internal/hub/upstream    — ok
internal/hub/usage       — ok
internal/relay           — ok
internal/relay/app       — ok
internal/relay/control   — ok
internal/relay/metering  — ok
internal/relay/runtime   — ok
pkg/platformid           — ok
```

`go vet ./...` — 无错误

### 前端 Vitest（全部 Green）

```text
6 test files, 19 tests, all passed
  src/api/client.test.ts         → 1 test
  src/api/workflow.test.ts       → 3 tests
  src/stores/workflows.test.ts   → 4 tests
  src/pages/LoginPage.test.ts    → 2 tests
  src/pages/UpstreamsPage.test.ts → 2 tests
  src/pages/ResourcesPage.test.ts → 7 tests
```

`tsc --noEmit` — 无类型错误

## 契约与管理

- **契约优先**：行为/字段变化先建立 failing contract test，再同步 OpenAPI → fixtures → codegen → 实现。Snapshot v1 在 C7 前视为 pre-freeze。
- **Admin Console**：实现以 `docs/admin-console-implementation.md` 为准；产品/UX 以 architecture 的 Admin Product Requirements 为准。
- **Runtime / Adapter**：Relay 保持 provider-agnostic。S0.1 证明 required capability profile 的完整执行链。
- **Testing / Freeze**：C6 使用真实 Admin build、Hub、Relay + deterministic Test Client/Test Adapter。C7 freeze manifest 至少 pin：architecture commit、platform-core commit、Client OpenAPI/fixture hash、Snapshot schema version、Admin build identity、qualification reference 和 scenario results。

## 当前不做

- Android S0.2 implementation
- Agent Space / S1
- S2/S3
- Relay WebSocket/realtime tunnel
- provider-specific body translation in Relay
- S1+/enterprise 空导航或通用 workflow/dashboard builder

## 下一步

**C4 Runtime Reference Profile**：建立 Test Client + Test Adapter，证明四个 runtime profile（SSE/Binary/Multipart/MCP）的完整执行链。
