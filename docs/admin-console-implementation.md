# Admin Console 具体实现规范

> 仓库：`topabomb/measix-platform-core`  
> 状态：S0.1 Concrete Implementation Authority  
> 架构基线：`topabomb/measix-architecture@6de9bfb794e60e9bb6c62501263cc1518e4f5ee3`  
> 产品/UX 权威：`docs/10-runtime-foundation/s0/measix-s0-admin-console-product-requirements.md`（architecture repo）  
> 组件架构：`measix-s0-admin-console.md`（architecture repo）  
> 测试规格：`measix-s0-admin-console-testing-spec.md`（architecture repo）  
> 文档职责：定义本仓库 Admin Console 实际代码结构、UI 设计系统、状态组织、具体依赖选择和 S0.1 实现落点。产品语义、wire 与安全不变量仍以上位 architecture/OpenAPI 为准。

## 1. 实现目标

S0.1 不是继续给现有占位页面补字段，而是把 `console/` 收敛成长期可演进的 MEASIX 管理平台前端。

当前实现已具备 Vue/Quasar shell、session、基础 API wrapper、Users、Upstreams、Resources、Releases、Usage、System 页面，但存在明确缺口：

- Resources 仍偏 Model-only；
- Upstream editor 仍有旧 `providerKind`/不完整 config；
- Usage 仍偏 model/request log 视角；
- 没有 canonical Snapshot Preview 产品面；
- 没有 Resource→Upstream→Runtime relationship view；
- 页面和 feature 结构还不足以承载完整 S0.1 editor。

本轮前端重构目标是解决这些缺口，同时避免为了“平台化”提前构建复杂 UI framework。

## 2. 技术基线

当前生产基线：

```text
Vue 3
TypeScript
Quasar / @quasar/app-vite
Vue Router
Pinia
openapi-typescript
native fetch wrapper
Vitest + @vue/test-utils
pnpm
```

Production：

```text
quasar build
→ console/dist/spa
→ Control Hub / Ingress same-origin static host
```

不增加 Node production backend，不从浏览器直连 Relay internal API。

## 3. 前端依赖策略

### 3.1 不设僵硬白名单

`console/package.json` 是具体依赖权威。允许按真实需求引入成熟 Vue/TypeScript/browser package，不要求每增加一个正常 frontend helper 都修改 architecture。

新增依赖需要满足：

1. 明确降低实现复杂度或显著改善交互/可视化/可访问性；
2. 与 Vue 3/TypeScript/Vite 生态成熟兼容；
3. 不建立第二套 business/wire authority；
4. 不与 Quasar 大面积重复；
5. bundle/runtime 成本与价值匹配；
6. 不自动持久化 Secret/token；
7. 有对应 feature/test 使用，不提交长期未使用 dependency。

### 3.2 S0.1 推荐新增依赖

以下是 S0.1 当前明确有实际用途的首选，不要求一次性全部安装；**在对应 feature 开始实现时与代码一起引入并锁定版本**。

| Package | 用途 | 决策 |
|---|---|---|
| `@vueuse/core` | resize、event、async/composable 等常见浏览器/Vue helper，减少自建 utility | 推荐引入 |
| `echarts` + `vue-echarts` | Overview/Usage 趋势、构成与可靠 cost/usage 可视化 | 推荐在首个 chart feature 引入 |
| `date-fns` | 时间范围、格式化、duration/relative time；避免散落手写日期逻辑 | 推荐在 Usage/Release timeline 引入 |
| `@vue-flow/core` | Resource→Upstream relationship/topology view | 条件引入：仅当 node-edge view 比 table/list 明显更清楚时 |

选择 ECharts 的原因：管理平台后续会持续需要 time-series、category comparison、usage/cost 等图表，ECharts 能覆盖这些而不引入第二套 UI framework。VueUse 用于 composable/browser helper，不拥有业务状态。Vue Flow 只服务关系图，不作为整个 Admin layout framework。

### 3.3 当前不默认引入

- **第二套 UI framework**：Element Plus、Vuetify、Ant Design Vue 等，不与 Quasar 并存；确需替换 UI framework 属架构/大规模迁移议题。
- **`@tanstack/vue-query`**：长期可能适合 read-heavy server state，但当前已经有 Pinia workflow stores + thin API wrapper。S0.1 先避免形成 Pinia/Query 双重 ownership；当分页缓存、background refresh、query invalidation 明显复杂时再评估。
- **通用 form framework/schema engine**：Hub/OpenAPI 是最终 authority。简单字段优先 Quasar validation；若复杂 dynamic form 明确出现，再选择成熟库。
- **Monaco/JSON editor**：S0.1 正常 authoring 不依赖 JSON；只因调试方便不引入。

### 3.4 版本策略

文档不硬编码所有 package patch version。具体版本由 `package.json` + `pnpm-lock.yaml` 固定。

新增/升级依赖时：

```text
pnpm add <package>
→ inspect lock diff
→ typecheck/test/build
→ bundle impact where material
→ commit package.json + lockfile + implementation together
```

## 4. 代码组织

目标不是一次性大迁移，而是随着 C1–C6 功能按 feature 收敛：

```text
console/src/
├── api/
│   ├── generated.ts
│   ├── client.ts
│   └── problem.ts
├── components/
│   ├── app/
│   ├── data/
│   ├── feedback/
│   └── forms/
├── composables/
├── stores/
│   ├── session.ts
│   ├── draft.ts
│   ├── activation.ts
│   └── operationalApply.ts
├── features/
│   ├── enrollment/
│   ├── upstreams/
│   ├── resources/
│   ├── release-review/
│   ├── pricing/
│   ├── usage/
│   └── system/
├── pages/
└── router/
```

规则：

- `pages/`：route composition、page-level loading/error/action；
- `features/`：业务 editor/workflow/visualization；
- `components/`：跨 feature 的纯 UI/feedback/data primitives；
- `stores/`：跨页面 workflow state，不为每个 input 建 store；
- `composables/`：可复用浏览器/交互逻辑，不复制 server business rule；
- `api/generated.ts`：生成物，禁止手改。

不要求为了目录好看把小组件过度拆碎；一个组件只有在职责可复用、可独立测试或明显降低 page 复杂度时拆出。

## 5. UI Shell 与视觉语言

### 5.1 Layout

使用 Quasar `QLayout/QDrawer/QHeader/QPageContainer`：

- desktop 默认左侧 drawer；
- drawer 支持 compact/collapse；
- narrow viewport 自动切换 overlay drawer；
- 页面内容使用统一最大阅读宽度策略，但 data table/chart 可占满 available width；
- 不使用 fixed pixel page width。

### 5.2 Page header

统一 `PageHeader` 或等效 pattern：

```text
Title
context/help text
optional status
primary action
secondary actions
```

避免每个页面重复手写不一致 header。

### 5.3 状态语义

统一 status mapping：

```text
positive: active/healthy/completed
warning: pending/degraded/partial
negative: failed/blocked/invalid
neutral: disabled/unknown/not-applicable
```

颜色必须配文本/icon；不要在 feature 内自行发明颜色语义。

### 5.4 Feedback primitives

保留/扩展：

```text
LoadingState
EmptyState
ProblemBanner
StatusChip
InlineFieldProblem
OperationPanel
```

`QNotify` 只用于低风险短消息；Publish/Apply/stale/degraded 必须页面内持久显示。

## 6. API 与 server-state

继续使用 generated OpenAPI types + `apiFetch` thin wrapper。

统一能力：

- same-origin credentials；
- mutation CSRF；
- `ApiProblem` typed mapping；
- request AbortSignal；
- cursor helper；
- no generic mutation retry；
- 401 session recovery；
- all command workflows retain Idempotency-Key。

Read-only page query 不应全部进入 Pinia。Pinia 用于 session、draft、activation、operational apply 等跨组件 workflow；普通 list/detail 优先 page/feature local query state。

## 7. Resources 实现

`ResourcesPage` 只作为 shell：

```text
Models | TTS | ASR | MCP | Policy
```

每个 tab 使用同一 editor layout：

```text
entity list
+ selected detail/editor
+ validation/status side information where useful
```

Desktop 可用 split view；窄屏切为 list → detail。

### Models

实现 `Provider/Model` editor，去除 `prv_placeholder`。Model 与 upstream binding 同一任务中可理解，但仍区分 logical identity 与 runtime execution section。

### TTS

必须有 voice field，profile 固定 `OPENAI_AUDIO_SPEECH` + MP3 baseline。

### ASR

明确 HTTP transcription；只暴露 model key/language/upstream/path 等字段。

### MCP

Streamable HTTP + `ENTERPRISE_MANAGED|NONE`。

### Policy

flags + default pickers；picker options 从当前 enabled draft resources 派生。

## 8. Upstream 实现

替换当前简化 create form。Editor 直接对应真实 `UpstreamConfig` executable contract：

```text
name
baseUrl
transportCapabilities
auth
correlationMode
usageCapabilityLevel
timeoutDefaults
```

UI 结构：

```text
Overview
Connection
Authentication
Capabilities
Timeouts (Advanced)
Verification / Revision
```

列表重点显示 status、candidate vs active、last test/apply；detail 使用 drawer 或 page，不用多层 dialog。

Test 结果结构化展示；Apply 显示 activation panel。

Secret input 使用 component-local `ref`；不得进入 Pinia/persistent storage。

## 9. Relationship View

S0.1 新增只读 `CapabilityRelationshipView`（命名可微调）：

```text
Resource
  → Upstream
  → candidate/active verification state
  → runtime path / transport
```

第一版优先使用 Quasar list/table + compact connection indicators；如果实体数量/关系复杂度使 node-edge 图明显更清晰，再引入 `@vue-flow/core`。

不要为了满足“可视化”强制图形化。可读性是要求，graph library 不是要求。

## 10. Validate / Review / Snapshot Preview

不把所有能力堆在 `ResourcesPage` 右侧 Validation card。

实现独立 review flow（route 或 large dialog/drawer 由实际 UX 决定）：

```text
Validate
→ structured errors/warnings grouped by resource
→ Change Summary
→ Client Snapshot Preview
→ Publish confirmation
```

Change Summary 使用 structured diff：Added / Changed / Removed / Policy / Runtime impact。

Snapshot Preview 只渲染 Hub 返回的 canonical client projection。推荐 tree/detail presentation，不引入 raw JSON 作为主体验。

## 11. Publish / Activation / Releases

`ActivationStore` 负责 command identity/recovery，不负责伪造产品阶段。

实现统一 `OperationPanel/ActivationTimeline`：

```text
VALIDATING
STAGING_RELEASE
APPLYING_RUNTIME
FINALIZING
ACTIVE / FAILED / DEGRADED
```

实际字段来自 server authoritative Activation；若 backend 只提供更粗状态，UI 只能展示有证据的阶段。

Release detail：metadata + change summary + activation history/timeline。

## 12. Overview / Usage 可视化

### Overview

组件结构建议：

```text
4–6 key metric/status cards
runtime consistency strip
usage/request trend
attention-needed list
recent activations
```

避免 10+ 同权卡片。

### Usage

从当前 model-centric request list 重构为：

```text
FilterBar
Summary metrics
trend visualization
resource-kind breakdown
request table
request detail drawer
```

Filters：Time/User/Resource/Kind/Upstream/Status/Completeness。

ECharts 只显示有意义的趋势/比较：request/error、token/audio/character semantic meter、reliable cost。UNKNOWN/PARTIAL 不填 0。

## 13. 时间与格式化

统一 formatter module/composable：

```text
formatDateTime
formatRelativeTime
formatDuration
formatBytes
formatCost
formatGeneration
formatStableId
```

推荐 `date-fns` 处理时间运算/格式化；不要在页面散落 `new Date(...).toLocaleString()` 和自写 duration math。

## 14. Accessibility / responsive

- native/Quasar form controls with visible label；
- visible keyboard focus；
- status text + icon；
- dialog/drawer focus trap 使用 Quasar 能力；
- chart 提供 text/table equivalent；
- dense table 在窄屏切 summary row/detail，而不是强制横向无限滚动；
- 主要 workflow 在 320px smoke 下仍可操作，但 desktop 是 S0.1 主要效率目标。

## 15. 测试落点

Component/unit test 与 feature 同目录或 `__tests__`，以职责清楚为准，不强制一种目录风格。

至少覆盖：

```text
Page shell states
Upstream editor payload + candidate/active
Secret non-persistence
all Resource editors
relationship view degraded/missing states
stale Draft
review diff
canonical Snapshot Preview
Activation recovery
Usage filters/kinds/unknown
chart data mapping unknown != zero
responsive/accessibility smoke
```

Browser E2E 在 `test/system/` 或现有 system harness 中运行 production build + real Hub/Relay，不能由 component test 替代。

## 16. 实施顺序

与 S0.1 C1–C6 对齐：

```text
A. shared UI/status/form primitives
B. complete Upstream editor + Test/Apply
C. Resources tabs + all editors + Policy
D. relationship view
E. Validate/Review/Snapshot Preview/Publish
F. Pricing
G. Usage/Overview/System visualization and filters
H. production browser E2E hardening
```

每一步按 Red → Green → Refactor；不要先做一次性“大前端重写”。

## 17. Definition of Done

Admin frontend implementation 只有达到下列才可标记 S0.1 complete：

- architecture Product Requirements 的所有 S0.1 Admin Exit 已有实现；
- no JSON/DB/internal API golden path；
- current OpenAPI/generated types 对齐；
- all four resource kinds + policy + pricing 可维护；
- relationship view 可理解 runtime mapping；
- Preview 来自 canonical server compiler；
- Apply/Publish refresh recovery 正确；
- Usage/Cost/UNKNOWN/PARTIAL 表达正确；
- production build + component tests + real-browser system E2E Green；
- 新依赖都有实际用途、lockfile 固定、无 Secret persistence 风险。
