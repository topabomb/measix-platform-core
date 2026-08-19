# Admin Console 实现规范

> Architecture baseline: `topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`  
> Product/UX: `measix-s0-admin-console-product-requirements.md`  
> Component architecture: `measix-s0-admin-console.md`

本文只定义 `console/` 的具体实现选择；产品功能和 UX 要求不在这里重复。

## 1. 技术基线

```text
Vue 3 + TypeScript
Quasar / @quasar/app-vite
Vue Router
Pinia
openapi-typescript
native fetch wrapper
Vitest + @vue/test-utils
pnpm
```

Production 为 `dist/spa` same-origin static build，不增加 Node production service，不从浏览器直连 Relay internal API。

## 2. 依赖策略

`console/package.json` + `pnpm-lock.yaml` 是具体依赖权威。正常前端依赖不需要 architecture 白名单。

当前推荐按实际 feature 引入：

| Package | 用途 |
|---|---|
| `@vueuse/core` | 常用 Vue/browser composable/helper |
| `echarts` + `vue-echarts` | Overview/Usage/Cost 趋势与比较 |
| `date-fns` | 时间范围、duration、relative time |
| `@vue-flow/core` | 仅当 Resource→Upstream 关系复杂到 graph 明显优于 table/list 时使用 |

原则：依赖必须有真实使用点、降低复杂度或明显改善交互；避免第二套完整 UI framework、重复业务/wire authority、无需求的大型 framework 和自动持久化 Secret/token 的插件。新增/升级依赖必须经过 typecheck/test/build，并提交 lockfile。

## 3. 代码组织

按职责组织，不追求目录数量：

```text
console/src/
├── api/          generated types + client/problem
├── components/   跨 feature UI primitives
├── composables/  可复用 browser/interaction logic
├── stores/       session/draft/activation 等跨页面 workflow state
├── features/     upstreams/resources/review/pricing/usage/system
├── pages/        route composition
└── router/
```

拆分条件：可复用、可独立测试或能显著降低 page/feature 复杂度。不要为每个 field 建 component/store/service。

## 4. UI primitives

优先形成少量稳定 primitives：

```text
PageHeader
LoadingState
EmptyState
ProblemBanner
StatusChip
InlineFieldProblem
OperationPanel / ActivationTimeline
FilterBar
```

Quasar 负责 layout/form/dialog/drawer/table 基础能力。高密度数据优先 table；详情优先 page/drawer/split view；避免多层 modal。

状态颜色统一为 healthy / pending-degraded / failed / neutral，并同时提供文本或 icon。

## 5. API 与状态

继续使用 generated Admin OpenAPI types + thin `apiFetch`：same-origin credentials、CSRF、typed Problem、AbortSignal、cursor pagination、401 recovery；mutation 不做通用自动 retry。

Pinia 只用于跨页面 workflow：session、draft、activation、operational apply 等。普通 list/detail 查询优先 feature/page local state，避免所有 server state 都进入全局 store。

Secret plaintext 只存在 component-local transient state，提交结束立即清理。

## 6. S0.1 feature 落点

### Upstreams

替换当前 `providerKind` 简化表单，直接编辑真实 UpstreamConfig：connection、auth/SecretRef、transport capabilities、correlation、usage capability、timeouts。列表清楚展示 candidate/active revision；Test 结构化展示 verification；Apply 使用统一 OperationPanel。

### Resources

`ResourcesPage` 作为 Models / TTS / ASR / MCP / Policy shell；各 editor 共享 list-detail/editor pattern。去除 `prv_placeholder`，把 logical resource 与 execution/binding 分区。TTS 必须有 voice；ASR 明确 HTTP transcription；MCP 只显示当前 Managed profile 支持的 auth ownership。

### Relationship view

第一版优先 table/list + compact connection indicators 展示 Resource → Upstream → runtime path/transport/status。只有实体/关系复杂后再使用 node-edge graph。

### Validate / Review / Preview

独立 review flow：validation → structured Added/Changed/Removed diff → canonical Client Snapshot Preview → Publish。Preview 只渲染 Hub 返回的 projection，不在浏览器重建 Snapshot。

### Publish / Releases

统一 activation recovery：同 command Idempotency-Key、poll authoritative Activation、刷新恢复同一 activation。Release detail 展示 metadata、change summary 和 activation history。

### Pricing

维护整个 PricingSet/expected revision；支持 create/edit/delete rule 和 stale 409。UNKNOWN/PARTIAL semantic meter 不计算假 cost。

### Usage / Overview / System

Usage 从 model-centric list 改为 resource-kind 视角，支持 required filters、request detail、semantic/cost completeness。ECharts 只用于有意义的趋势/比较，UNKNOWN/PARTIAL 不补 0。Overview 保持少量关键状态 + attention list；System 以只读诊断为主。

## 7. Formatting / responsive / accessibility

集中提供 datetime/relative time/duration/bytes/cost/stable ID formatter，避免页面散落重复格式化逻辑。

Desktop 是主要效率目标；窄屏仍应完成核心操作。Focus 可见，状态不只依赖颜色；图表提供 text/table equivalent。

## 8. 测试

Unit/component 覆盖 payload/state/error/recovery，不复制 Hub business validator。重点包括：

```text
Upstream candidate/active + Secret non-persistence
all Resource editors
relationship view missing/degraded states
stale Draft
review diff + canonical Preview
Activation recovery
Pricing stale/unknown
Usage filters/kinds/completeness
chart mapping UNKNOWN != 0
responsive/accessibility smoke
```

Production browser E2E 在 system harness 中使用 real Hub/Relay，不能由 component tests 代替。

## 9. 实施顺序

随 S0.1 C1–C6 推进：shared primitives → Upstreams → Resources → relationship view → Review/Preview/Publish → Pricing → Usage/Overview/System visualization → browser E2E hardening。
