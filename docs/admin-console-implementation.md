# Admin Console 实现规范

> Architecture baseline: `topabomb/measix-architecture@02ba0add27cddce3bcebe63433495df6ea39b9ad`  
> Product/UX: `measix-s0-admin-console-product-requirements.md`  
> Component architecture: `measix-s0-admin-console.md`

本文只定义 `console/` 的具体前端实现基线。产品功能、状态语义和 S0.1 Exit 要求仍以上游 architecture 文档为权威；这里关注如何把当前 S0.1 做成一套可以自然演进到 Agent Space、Agent Runtime、Runtime Hook 和后续企业治理的长期管理界面，而不是一次性 CRUD 页面。

## 1. 长期设计目标

Admin Console 的前端形态参考成熟 cloud console 的共同经验，但不复制某一家产品：**稳定 App Shell、紧凑的信息密度、按任务组织导航、列表与详情快速切换、上下文工具面板、长期操作持续可见、移动端保留核心操作能力**。

从 S0 开始固定以下原则：

1. **Shell 稳定，业务域演进**：后续新增 Agent Space / Agent / Run / Hook / Governance 时，新增导航域和页面，不重写全局布局。
2. **任务优先于站点地图**：一级导航只放管理员经常进入的领域；Create/Edit/Run detail 等页面从领域页面进入，不把所有 route 都塞进导航。
3. **高信息密度但不拥挤**：管理页优先 table、compact list、split detail、structured form；避免大面积 hero、卡片墙、装饰图表和无意义留白。
4. **主工作区优先**：详情、诊断、帮助等次要信息进入 contextual panel/drawer；不能让常驻辅助区域挤压主任务。
5. **状态始终可恢复**：Apply/Publish 等长操作以及未来 Agent Run 都不能依赖 toast；刷新、返回、跨页后仍能重新定位权威状态。
6. **自适应不是“缩小桌面版”**：桌面追求效率，移动端按优先级重排信息和操作，但不创造第二套业务逻辑。
7. **不为未来造空抽象**：只预留稳定布局/路由/状态边界，不创建 S1/S2 空页面、空 feature 或通用 workflow engine。

## 2. 技术基线与依赖策略

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

`console/package.json` + `pnpm-lock.yaml` 是具体依赖权威。正常前端依赖不需要 architecture 白名单；依赖必须解决真实问题并经过 typecheck/test/build。

当前/后续按 feature 需要优先考虑：

| Package | 用途 |
|---|---|
| `@vueuse/core` | 稳定 browser/composable helper |
| `echarts` + `vue-echarts` | Usage/Cost/Runtime 等真实趋势和比较 |
| `date-fns` | 时间范围、duration、relative time |
| `@vue-flow/core` | 仅当关系复杂到 graph 明显优于 table/list 时使用 |
| `@playwright/test` | S0.1 real-browser E2E 与长期关键操作路径 |

Server query/cache 暂不强制引入第二套 framework。页面通过 feature composable 隔离 query/mutation orchestration；当 S1/S2 出现大量跨页缓存、background refresh 和 invalidation 后，可在该边界内引入 `@tanstack/vue-query`，而不是让页面直接依赖新的 server-state 模型。

禁止：第二套完整 UI framework、自动持久化 Secret/token 的插件、没有当前使用点的大型 abstraction framework、与 generated OpenAPI 并行维护的手写 DTO authority。

## 3. App Shell：从 S0 就按长期管理平台构建

长期 Shell 使用一个 `QLayout`，只允许一个主导航和一个 contextual secondary surface，避免嵌套 layout 体系。

```text
┌──────────────── Global Header ────────────────────────────┐
│ MEASIX / context       search*   health  operations  user │
├───────────────┬───────────────────────────────┬───────────┤
│ Primary Nav   │ Main Route Workspace          │ Context   │
│ collapsible   │                               │ Panel*    │
│               │ breadcrumbs / page header     │           │
│               │ toolbar / content             │           │
│               │                               │           │
└───────────────┴───────────────────────────────┴───────────┘

* 只有有真实能力时显示，不做空占位。
```

### 3.1 Global Header

Header 只承载跨领域能力：

- 产品/当前 Deployment context；
- 全局高优先级 health/degraded indicator；
- operation/notification 入口；
- 当前管理员与 session actions；
- 后续资源搜索/command palette、Admin AI assistant 等全局工具的入口。

S0 没有全局搜索或 AI assistant 后端时不显示占位按钮。未来多 Organization/Tenant context 也应扩展同一个 context slot，而不是增加第二条导航栏。

### 3.2 Primary Navigation

S0.1 仍严格使用当前产品定义的：

```text
Overview
Users
Resources
Upstreams
Releases
Usage
System
```

实现上改为 **route metadata / navigation registry 驱动**，不要把导航数组硬编码在 `AdminLayout.vue`。Registry 至少包含稳定 route id、label、icon、order、visibility；权限/阶段能力出现后再增加对应 visibility 条件。

未来项目规模扩大后允许按领域形成最少量 group，例如 Access、Capabilities、Agents、Operations，但只有 group 中存在真实页面时才显示。Agent Space / Agents / Runs / Hooks 到对应阶段再加入，S0 不出现 coming-soon 项。

### 3.3 Page Header

所有业务页使用一致的 page header：

```text
Breadcrumbs when deep route
Title + concise context
authoritative status / identity summary
primary action + secondary actions
```

页面主操作位置保持稳定。窄屏时只保留一个主要操作，其余进入 overflow menu，避免标题区横向溢出。

### 3.4 Context Panel / Inspector

右侧 secondary panel 用于**当前上下文的次要信息**，例如：

- list selected item quick detail；
- validation/diagnostic detail；
- Activation/operation progress；
- future Agent Run metadata / artifacts / diagnostics；
- future Admin assistant/help。

同一时刻只打开一个 contextual surface。Desktop 可以 side panel；空间不足时自动转 overlay/bottom/full-screen surface。不要在业务页内叠多层 drawer + modal。

### 3.5 Persistent Operations Surface

Shell 预留一个长期操作入口。S0 先展示 Apply/Publish/Revoke 等 server-side Activation；以后 Agent Run、Agent Space provisioning 等仍由各自领域模型负责，但可以共享“当前有长期操作正在运行/失败”的全局入口。

**共享 surface 不代表共享领域模型**：`Activation`、未来 `AgentRun`、`AgentSpaceOperation` 不合并成万能 Operation entity。

## 4. 三种稳定页面形态

不要为每个 feature 发明独立页面结构。长期只维护三种主要 layout pattern。

### 4.1 Collection — 资源集合

用于 Users、Resources、Upstreams、Releases、Usage request 等：

```text
PageHeader
Filter/Search toolbar
Table / compact list
Pagination
optional selected-item inspector
```

桌面优先 table；行点击进入 detail 或打开 inspector。Filter、sort、page、tab 等可恢复视图状态优先写入 URL query，使 refresh/back/forward/deep link 行为可预测。

不要把所有 collection 都改成 card。只有实体很少、视觉摘要比列比较更重要时使用 compact cards/list。

### 4.2 Detail / Edit — 资源详情与配置

用于 Upstream、User、Managed Resource、Policy、Release 等：

```text
PageHeader + status
summary / tabs or sections
main form/detail
advanced / diagnostics section
sticky-or-visible action area when needed
```

简单编辑使用单页 section；只有步骤间确有依赖、信息量明显超过单页可理解范围时才使用 wizard。Create/Edit 正常情况下不是一级导航项。

### 4.3 Workbench — 未来 Agent / Workspace 长时工作区

S1/S2 后 Agent Space、Agent Run、远程 Agent 调试会需要和 CRUD 完全不同的持续交互空间。现在不实现，但 Shell 必须允许：

```text
history / run list      main work area          inspector
                        chat / events           run state
                        artifacts               usage
                        workspace view           diagnostics
                        future terminal/log
```

Workbench 允许使用更大可视区域、可收起侧栏和连续事件流，不受普通 form 最大宽度限制。它仍复用同一 Global Header、Primary Navigation、status/operation semantics；因此未来增加 Agent 场景时不需要另做一个“第二管理后台”。

## 5. 响应式与信息密度

只维护三种逻辑 layout mode，由 Quasar breakpoint + CSS 统一决定，不允许每个页面自己定义一套设备判断：

| Mode | 典型行为 |
|---|---|
| **Wide** | 左导航常驻；主内容高密度；必要时右侧 inspector 并排 |
| **Compact** | 左导航可折叠/图标态；inspector 默认 overlay；降低并排列数 |
| **Mobile** | 主导航 overlay；单列内容；detail/editor 全宽；secondary action 进入 overflow |

具体 px 阈值使用 Quasar breakpoint 统一维护，不在业务组件散落 magic number。

### Desktop / Wide

- collection/table 使用可用宽度，不强制窄 content max-width；
- 普通表单保持可读最大宽度，避免字段横跨超宽显示器；
- list → detail 高频场景优先 split/inspector，减少反复跳页；
- toolbar、filter、status 使用 compact spacing，主要操作保持明显。

### Mobile

- 核心操作必须可完成，但不追求一次展示全部诊断列；
- table 优先隐藏低优先级列、提供 row detail；只有确实更清楚时才转换 compact list；
- 多列表单变单列；高级配置折叠；
- detail inspector 变全屏/页面，而不是保留狭窄右栏；
- destructive/submit 等关键动作保持可触达，可使用 sticky action area；
- 不使用 hover-only action；交互 target 满足触控尺寸；
- charts 单列显示并保留精确数据入口。

响应式代码首先通过 CSS/Grid/Flex 和组件 slot 解决，只有交互模型真正变化时才根据 layout mode 分支。Desktop 和 Mobile 不维护两套业务组件。

## 6. 视觉语言与主题边界

继续使用 Quasar 作为 UI foundation，不自建完整 design system。额外只维护一层很薄的 MEASIX semantic tokens / utility classes：

```text
page/content gutter
surface / border / muted text
compact control/table density
healthy / pending / degraded / failed / neutral
focus / selected / interactive state
```

当前页面中散落的 `bg-grey-*`、`text-grey-*`、`bg-green-*` 不应长期承担业务语义；状态和 surface 应逐步收敛到 semantic class/token，使以后 dark mode 或品牌调整不需要逐页修改。

视觉约束：

- 以 border、spacing、typography 建立层级，少用重阴影；
- 状态色克制，并始终伴随文本/icon；
- 关键指标用少量 status/metric card，不做 dashboard card wall；
- 不使用营销型 hero、渐变大标题和装饰动画；
- default density 偏紧凑，移动端恢复合理 touch spacing；
- stable ID/hash/revision 等机器信息使用易扫描、可复制的 secondary text/monospace 表达。

## 7. 代码组织与边界

保留 Quasar 的自然工程结构，只增加真正有价值的 feature 层，不引入 Clean Architecture 式前端目录爆炸：

```text
console/src/
├── api/          generated types + transport/problem
├── boot/         Quasar boot integration only
├── components/   跨 feature UI primitives/pattern pieces
├── composables/  跨 feature browser/workflow helpers
├── css/          Quasar variables + thin semantic tokens
├── layouts/      App Shell / responsive layout orchestration
├── features/     有真实复杂度的业务域实现
├── pages/        route composition; 不承载大型业务 workflow
├── router/       routes + route metadata/navigation registry
└── stores/       session / cross-route operation/workflow state
```

`features/` 按业务能力而不是按技术层拆分。S0.1 随实际实现逐步出现：

```text
features/
  identity/
  upstreams/
  resources/
  releases/
  pricing/
  usage/
  system/
```

未来只在对应阶段增加 `spaces/`、`agents/`、`runs/`、`hooks/` 等，不预建空目录。

Feature 内部**默认保持扁平**。只有出现多个独立 editor/workflow 或文件数量明显影响理解时，再增加 `components/`、`composables/` 等子目录。不要强制每个 feature 都拥有 `api/service/repository/model/store` 五层，也不要为每个 field 建 component/store/service。

### 拆分规则

抽取 component/composable/store 至少满足一个条件：

- 两处以上真实复用；
- 有独立、稳定的交互/状态边界；
- 可以独立测试并显著降低页面复杂度；
- 跨 route 需要保持 workflow state。

否则代码留在 feature/page 附近更容易维护。

## 8. UI primitives 与交互 pattern

优先形成少量稳定 primitives，而不是组件目录膨胀：

```text
PageHeader
LoadingState / EmptyState
ProblemBanner / InlineFieldProblem
StatusChip
FilterBar
OperationPanel / Timeline
ResourceTable or collection wrapper when repetition真实存在
InspectorPanel / DetailDrawer when repetition真实存在
```

Quasar 继续负责 layout/form/dialog/drawer/table/tab 基础能力。

统一交互约定：

- list/detail 页面保持 URL 可恢复；
- primary action 每页最多一个视觉主按钮；
- destructive action 有明确确认和对象身份；
- modal 用于短、局部、阻断式确认，不用于复杂编辑；
- server validation 显示在字段/对象附近，ProblemBanner 保留全局上下文；
- stale local edit 不静默覆盖；
- async command 依赖权威 terminal state，不把 `202` 渲染为成功；
- precise detail 可展开，不把高级字段永久铺满默认视图。

## 9. API、Server State 与 Workflow State

继续使用 generated Admin OpenAPI types + thin `apiFetch`：same-origin credentials、CSRF、typed Problem、AbortSignal、cursor pagination、401 recovery；mutation 不做通用自动 retry。

状态分成三类，不混在一个全局 store：

1. **Server resource/query state**：列表、详情、summary，优先 feature/page composable local state；
2. **Cross-route workflow state**：session、draft、activation/operational apply 等，Pinia；
3. **Transient sensitive state**：Secret plaintext 等，只在 component-local memory，提交/关闭后立即清理。

未来若引入 query cache framework，必须替代重复 server-state boilerplate，而不是与 Pinia 再维护一份同源 cache。

轮询和 background refresh 统一考虑：页面不可见时降频/停止；切回页面后先重新读取 server authority；Agent Run 等未来高频事件只有出现真实协议后再决定 SSE/WebSocket，不因为前端需要“实时感”提前创造 wire。

## 10. S0.1 feature 的实现落点

本节只记录实现组织，不重复 Product Requirements。

### Upstreams

替换当前 `providerKind` 简化表单，使用真实 `UpstreamConfig`。Collection + Detail/Edit pattern；connection/auth/capability/timeouts 分 section，Advanced 默认折叠。Test result 使用 inspector/inline diagnostic；Apply 进入统一 operation surface。

### Resources

`ResourcesPage` 负责领域入口和 Models/TTS/ASR/MCP/Policy 切换；具体 editor 落 `features/resources/`。共享 Identity / Capability / Binding / Advanced 的视觉骨架，但各 capability 保持自己的 typed fields，不做万能 dynamic form engine。

### Relationship View

第一版使用紧凑 table/list + connection/status indicator；当资源数量和关系复杂度证明 graph 更有效后，再引入 node-edge view。该视图是 projection，不创建第二套 editable state。

### Validate / Review / Preview / Publish

作为明确的 review workflow 组织，但不要做成多层 modal。Validation issue 能深链回对应 resource/tab；structured diff 和 canonical Client Snapshot Preview 共处 review workspace；Publish 后切换到持久 OperationPanel/Activation detail。

### Releases

Collection → Detail pattern。详情集中 metadata、diff summary、snapshot identity、activation history/timeline，避免在列表行塞入全部诊断信息。

### Pricing / Usage

Pricing 是结构化 editor，不做通用 rule-builder。Usage 是长期运营 collection/analysis 页面：FilterBar + summary/trend + request table/detail；resource kind 和 completeness 是一等维度。ECharts 只表达有意义趋势，UNKNOWN/PARTIAL 不补零。

### Overview / System

Overview 保持少量关键状态和 attention list；System 是诊断详情。两个页面不能重复成为两份“大而全 dashboard”。

## 11. 为 Agent 场景保留的明确接口，不提前实现业务

中长期 Admin Console 会新增 Agent Space、Agent Definition/Run、Runtime Hook 等领域。现在只保证以下前端结构不会阻碍它们：

- navigation registry 可增加新的真实领域组；
- Shell 支持 contextual inspector 和 persistent operation indicator；
- Workbench 页面可占满主内容区并收起两侧 panel；
- URL/deep-link 能定位 `spaceId/agentId/runId` 等稳定实体；
- Usage/Cost 的过滤和 detail pattern 可自然加入 Agent/Run/compute/storage 维度；
- global context slot 可在未来承载 Organization/Tenant scope；
- responsive model 能把 workbench side panel 降级为 mobile full-screen detail。

不要现在把 `Activation` 泛化成 Agent Run，也不要把 Managed Resource editor 泛化成 Agent Definition builder；它们只是复用 Shell、collection/detail/workbench pattern 和基础 UI primitives。

未来若增加面向管理员的 AI assistant，它属于 **global contextual tool**；受管 `Agent Definition` / `Agent Run` 属于平台业务资源。二者在信息架构和状态模型上保持分离。

## 12. Formatting、Accessibility 与可恢复导航

集中提供 datetime/relative time/duration/bytes/cost/stable ID formatter，避免页面重复格式化逻辑。

必须保持：

- keyboard-visible focus；
- 状态不只依赖颜色；
- icon-only action 有 accessible label；
- chart 有精确数值/table/detail 等等价入口；
- list filters/tab/selection 在合理范围内可通过 URL 恢复；
- browser back/forward 不丢失用户所在领域和上下文；
- mobile 不存在 hover-only 信息；
- long-running operation refresh 后可重新发现。

## 13. 测试基线

Unit/component 不复制 Hub business validator，重点验证 payload、state projection、error、recovery 和 layout behavior。

除现有 S0.1 feature tests 外，前端基础设施至少增加以下长期稳定测试：

```text
navigation registry → correct route/active state
wide / compact / mobile shell behavior
mobile navigation + inspector fallback
page header primary/overflow action behavior
URL-restored filters/tab/selection
operation refresh/recovery
no Secret persistence
no browser → Relay/internal API
```

Feature component tests 继续覆盖 Upstream、Resources、Review/Preview、Pricing、Usage 等 architecture Testing Spec 场景。

Real-browser E2E 使用 production `dist/spa` + real Hub/Relay system harness。代表性 viewport 至少覆盖 desktop 与 mobile smoke；关键路径测试关注可操作性和语义，不建立脆弱的大规模 pixel-perfect screenshot suite。视觉回归只用于少量 Shell/高风险 layout。

## 14. 实施顺序

当前不先做“完整设计系统”，而是随 S0.1 产品闭环把长期骨架落地：

```text
1. App Shell / navigation registry / semantic styling / responsive modes
2. shared page header + collection/detail/inspector patterns
3. Upstreams
4. Resources + Policy + relationship view
5. Review / Preview / Publish / Releases
6. Pricing + Usage
7. Overview / System
8. production browser E2E + responsive/accessibility hardening
```

每一步都只抽取已经出现真实复用的 primitive。完成 S0.1 后，S1/S2 新页面优先复用这三个页面模型和同一个 Shell，而不是建立新的前端架构。