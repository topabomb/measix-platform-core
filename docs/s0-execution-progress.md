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
| C2 Managed Resource Editor | ✅ Green | 五类 editor 改为 Tab 结构 + Runtime Binding（upstream select/transport policy）+ relationship view；Models/TTS/ASR/MCP/Policy 均可绑定真实 upstream |
| C3 Snapshot Projection & Preview | ✅ Green | `POST /api/admin/v1/draft:preview` 端点 + PreviewDraftRequest/Response schema + handler + service + UI dialog + TDD 1 test Green |
| C4 Runtime Reference Profile | ✅ Green | Test Adapter + Test Client 四 profile 完整闭环（Chat req/resp+SSE、TTS binary、ASR multipart、MCP Streamable HTTP、timeout/cancel/4xx/5xx）+ real relay system-smoke + CAP-C4-040..045 admission（stale gen/invalid JWT/revoked session/disabled user/unknown resource/internal header spoof）|
| C5 Usage / Pricing / Observability | ✅ Green（Admin） | Summary 完整指标（Requests/Blocked/semantic meters/Usage completeness EXACT/PARTIAL/UNKNOWN）；完整 filters（time range + userId/resourceId/resourceKind/upstreamId/status/completeness，live watch）；Request detail 对话框（identity/generation/status/duration/bytes，不含 prompt/body/Secret）；Pricing editor（GET/PUT /api/admin/v1/pricing，乐观并发）；Overview 新增 last Activation / control-not-converged / Upstream 汇总；SystemPage applied controlRevision+bundle hash+convergence |
| C6 Browser + Hub + Relay System E2E | ⏳ 部分 | real relay system-smoke（CAP-C4 四 transport + admission）已 Green；缺 Hub 全链路 + real browser 的 S0.1 Gate 证据 |
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

5. **Admin Usage filter/read model 补齐（P1-C，head `d5a6dfa`）**：
   - `/usage/summary` 与 `/usage/requests` 新增可组合 query filters：from/to、userId、resourceId、resourceKind、upstreamId、status、completeness
   - `resourceKind`（PROVIDER/MODEL/TTS/ASR/MCP）与 `status`（SUCCESS/ERROR/BLOCKED）为 frozen enum
   - backend `ListRequests`/`Summary` 改为接受 `usage.Filter`，ent 谓词组合过滤（status 语义：SUCCESS=forwarded&<400，ERROR=forwarded&>=400，BLOCKED=!forwarded）
   - UsagePage 增加 user/resource/kind/upstream/status 过滤器与 active filters 展示，修正 request 列表字段
   - 新增 4 个契约测试 + 1 个 `ListRequests` filter 单元测试，全部 Green
   - 注：新增共享枚举值导致 oapi-codegen 对 `PROVIDER/ERROR/WARNING` 等重命名带类型前缀（`ReleaseDiffKindPROVIDER` 等），相关代码已同步更新

### Frontend-first 长期骨架（实施顺序第 1 步，head `f4f7b1e`）

1. **Navigation registry**（`src/router/navigation.ts`）：
   - 由 route metadata 驱动，不再硬编码在 `AdminLayout.vue`
   - 7 项 S0.1 IA：Overview/Users/Resources/Upstreams/Releases/Usage/System（stable id/label/icon/order/visibility）
2. **响应式 App Shell**（`AdminLayout.vue`）：
   - Wide（>md）常驻 drawer；Compact（md）可折叠 mini；Mobile（<md）overlay drawer
   - 单一 QLayout，Global Header + Primary Nav + PageContainer
3. **PageHeader** 组件：breadcrumbs、title+context、semantic status、primary+secondary actions（窄屏 secondary 进 overflow）
4. **Semantic status/style**：
   - `StatusChip` 收敛为 healthy/pending/degraded/failed/neutral 语义色调，始终带文本
   - `HealthIndicator`：Global Header 常驻 runtime degraded/Relay not ready 高优先级指示
   - `useSystemHealth` composable：模块级共享轮询 system/status
5. Overview/System 页面接入 PageHeader
6. 新增 navigation registry 单元测试（3 tests），console 22 tests Green

### C2/C3 Resources 契约修正（head `7419df0`）

1. **Publish 契约闭环**：`publish()` 现在发送 `expectedDraftRevision` + `acknowledgedWarningCodes`（对 validate warnings 显式确认），不再发送空 body
2. **移除 `prv_placeholder`**：新增 provider 创建/编辑/删除（候选 id，绝不用占位 provider）；`Add model` 在无 provider 时禁用，有 provider 时绑定第一个真实 provider
3. provider 删除有引用保护（被 model 引用时拒绝）
4. 新增 2 个 ResourcesPage 测试（real-provider model 添加、publish body 契约），console 24 tests Green

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

### Admin Console UI 布局与工作流收敛（frontend-first）

1. **响应式 App Shell 修正**（`AdminLayout.vue`）：
   - 修复 Compact(md) 抽屉 mini 状态逻辑，`mini` 仅在未展开时生效，`mini-to-overlay` 保证展开回填
   - Mobile(<md) 导航点击后自动关闭 overlay drawer（route 变化 watch 驱动，实施 §5/§12）
   - Wide 页面内容容器居中并限制 max-width 1280px，兼顾极宽屏可读性（实施 §5 Desktop/Wide）
   - 新增登录身份菜单（Sign out）与 Mobile 独立退出按钮

2. **ResourcesPage 重构为 Tab 结构**（product §8）：
   - 一级 Tab：Overview | Models | TTS | ASR | MCP | Policy
   - Overview 含 Providers 管理与 **Resource→Upstream relationship view** 表格（kind/resource/upstream/status/启用态）
   - 每个 resource editor 新增 **Runtime Binding**：Upstream select（active 优先）+ transport policy（model=SSE、tts=Binary、asr=Multipart、mcp=Request-Response），复用稳定 runtimeRouteId
   - `draft.ts` 新增 `bindingFor/setBinding/removeBinding`，binding 引用空 upstream 即删除，候选 `rte_*` id 跨编辑稳定
   - 保留 Validate/Preview/Publish 工作流与 warning acknowledgement

3. **Upstreams 增加 inline Secret 创建**（C1 闭环）：
   - 新增 "Create secret" 对话框（name+value，write-only），调用 `POST /api/admin/v1/secrets`
   - 创建成功后自动回填 create-upstream 的 auth SecretRef（secretId/version），满足"无需 JSON/API 完成 Secret+Upstream"

4. **页面标题一致化**：Users/Releases/Usage/Upstreams 改用 `PageHeader`（breadcrumbs/title/subtitle/actions），与 Overview/System 对齐
5. **网格响应式修正**：System/Usage 的 `col-3` 硬编码改为 `col-xs-12 col-sm-6 col-md-3`，避免移动端挤压
6. 新增/更新测试：AdminLayout 3 tests、ResourcesPage 9 tests、UpstreamsPage 3 tests（含 secret 创建），console 32 tests Green；`tsc --noEmit`、production build 均 Green

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
12 test files, 51 tests, all passed
  src/api/client.test.ts          → 1 test
  src/api/workflow.test.ts        → 3 tests
  src/router/navigation.test.ts   → 3 tests
  src/stores/workflows.test.ts    → 6 tests
  src/layouts/AdminLayout.test.ts → 3 tests
  src/pages/LoginPage.test.ts     → 2 tests
  src/pages/UpstreamsPage.test.ts → 3 tests
  src/pages/ResourcesPage.test.ts → 10 tests
  src/pages/UsagePage.test.ts     → 12 tests
  src/pages/PricingPanel.test.ts  → 3 tests
  src/pages/OverviewPage.test.ts  → 3 tests
  src/pages/SystemPage.test.ts    → 2 tests
```

`tsc --noEmit` — 无类型错误；`quasar build` production dist/spa 构建成功

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

## 已完成的变更清单（追加：CAP-C4 admission/cancel system-level 覆盖）

1. `test/system/client`：Test Client 新增 `SpoofHeaders` 注入（用于验证 inbound sanitization），并新增：
   - `TestCAPC4042RevokedSessionRejectedNoForward`（401 invalid_session，无 forward）
   - `TestCAPC4042DisabledUserRejectedNoForward`（403 user_disabled，无 forward）
   - `TestCAPC4043UnknownResourceRejectedNoForward`（403 resource_not_allowed，无 forward）
   - `TestCAPC4045ClientInternalHeaderSpoofStripped`（伪造 X-Measix-Request-Id / X-Measix-Internal / X-Forwarded-For 被 Relay 剥离，且不 reach upstream）
   - `TestCAPC4022ClientStreamCancelPropagates`（client cancel 传播到 upstream）
2. `test/system/adapter`：`RequestFact` 新增 `Headers`（`safeHeaders` 只记录非敏感 header，排除 Authorization/Cookie），`XMeasixRequestId` 已有。
3. `env` helper 支持 `revokeSession()`/`disableUser()`（重放 control state，ControlRevision 递增避免 hash conflict）。

### Admin Console C5 增强（frontend-first）

1. **UsagePage**：
   - 新增 `kindOf()` 从稳定 resourceId 前缀（mdl_/tts_/asr_/mcp_）把 request 分类为 MODEL/TTS/ASR/MCP，并在 ledger 行显示彩色 kind chip
   - ledger 行新增 errorClass（negative chip）、durationMs、upstreamHttpStatus
   - Cost 卡片新增语义状态 chip（cost KNOWN/PARTIAL/UNKNOWN，颜色区分）
   - 改为 Summary | Pricing 双 Tab 结构
2. **PricingPanel（新组件，product §C5 Pricing editor）**：
   - 读取 `GET /api/admin/v1/pricing` → `PricingSet`（revision + rules）
   - 可增删定价规则（meter 下拉枚举标准 meter、unitSize/unitPrice/currency/effectiveFrom）
   - `PUT /api/admin/v1/pricing` 提交，携带 `expectedPricingRevision` 实现乐观并发，成功后以服务端返回的 revision/rules 为准
   - 对 `rules` 空数组做防御处理（`?? []`），避免渲染崩溃
3. 新增/更新测试：UsagePage 6 tests、PricingPanel 3 tests；console 共 **10 files / 40 tests** 全 Green；`tsc --noEmit`、production build 全 Green

### Admin Console C5 第二轮完成（§14/§15，frontend-first，TDD）

1. **UsagePage Summary 完整指标（§14 Summary）**：
   - Blocked 计数卡（requestCount - forwardedRequestCount）
   - Semantic meters 卡展示 meter+quantity+confidence，按类别着色（token/characters/audio）
   - Usage completeness 卡：EXACT/PARTIAL/UNKNOWN 计数
2. **Filters 完整（§14 Filter）**：
   - 新增 `completeness` 筛选器（EXACT/PARTIAL/UNKNOWN），query 携带
   - Range 快捷下拉（Last 24h/7d/30d）+ Reset 清空
   - 筛选变更 watch 自动 live refresh
3. **Request Detail 对话框（§14 Request Detail）**：
   - 点击 ledger 行打开，展示 requestId/interactionId/User/Device/Resource/Upstream/RuntimeRoute/Generation/ControlRevision/status/duration/bytes/errorClass
   - 明确不显示 prompt/body/Secret
4. **Overview/System 可观测性（§15）**：
   - Overview 新增 last Activation（state chip）、control-not-converged 告警、Upstream active/degraded/disabled 汇总、Relay last seen
   - SystemPage Relay 卡新增 applied controlRevision + bundle hash + convergence 标记
5. 测试：新增 OverviewPage 3 tests、SystemPage 2 tests、UsagePage 扩展至 12 tests；console 共 **12 files / 51 tests** 全 Green；`tsc --noEmit`、production build 全通过。任务清单见 docs/c5-task-tracking.md

## 下一步

**C6 Browser + Hub + Relay System E2E（Golden Path）**：建立真实 Admin build（dist/spa）+ Control Hub + Runtime Relay + SQLite/migrations + deterministic Adapter/Test Client 的 Golden Path 全链路系统 E2E，覆盖 Login→User→Secret/Upstream→Resources（五 tab + binding）→Pricing→Validate/Review→Publish→Activation→Relay runtime traffic→Usage/Cost/System。其中 real-relay system-smoke 与 hub→relay control 交接已分别 Green，缺统一双进程全链路 + real-browser Playwright E2E。

**C7 S0.1 Freeze**：仅当 C0–C6 全 Green 后执行，记录 architecture commit / platform-core commit / Client OpenAPI hash / fixture hash / schemaVersion / Admin build identity / system scenario / real Adapter qualification。
