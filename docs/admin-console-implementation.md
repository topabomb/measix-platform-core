# Admin Console 实现规范

> Architecture authority：[阶段阅读清单](../../measix-architecture/docs/measix-stage-document-index.md)；验收时 pin exact architecture commit
> Product/UX：`measix-s0-admin-console-product-requirements.md`  
> Required tests：Admin Testing Spec + 当前子阶段 CAP / ERX / ETG System Testing Specs；不再引用已移除的独立 Admin component 文档。

本文只定义 `console/` 的**具体实现约束和当前落地事实**。导航、用户任务、visual authoring、Review/Publish、状态语义和 S0.1 Exit 由 architecture 仓库定义；本文不维护第二份产品/UX 权威。

## 1. 技术与运行边界

当前前端基线：

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

- `console/package.json` 与 `console/pnpm-lock.yaml` 是前端依赖/版本权威；
- production 输出为 `dist/spa`；same-origin 静态托管是发布要求，Hub main 通过 `--admin-assets-dir` 配置既有 static handler，部署/同源路由见 `docs/operations.md`；
- 浏览器只调用 Control Hub Admin API，不直连 Relay internal API；
- API DTO/type 来自 generated Admin OpenAPI；不维护平行手写 wire model；
- Secret plaintext 不进入 localStorage、持久 Pinia state、日志或测试 artifact。

Root repository 的 npm orchestration、实际开发命令与 system harness 生命周期由 `docs/development.md` 维护，不在本文重复。

## 2. 当前源码组织

`ResourcesPage` 使用统一配置工作台组织 Overview、Models、Image Generation、TTS、ASR、MCP、Assistants 和 Policy。Image Generation 是独立受管资源，只表达同步 text-to-image、允许尺寸、单次数量上限和固定 Runtime binding，不借用 Model 或引入资源级额度。协议选择只包含 `OPENAI_IMAGES_GENERATIONS` 与 `DASHSCOPE_MULTIMODAL_GENERATION`；切换协议会原子改写其固定 Runtime path、默认 canonical 尺寸和 binding protocol，避免产生跨协议半配置。桌面显示固定分区导航，窄屏使用同一 section state 的选择器；Policy 直接编辑当前五项必填用户配置准入开关和各类默认资源。新草稿五项默认 false；不存在旧策略采用按钮或缺字段补齐逻辑，非当前旧草稿随旧开发数据库清理。服务端在 HTTP 边界独立校验五项必填 Boolean 与所有资源、Assistants/Starters 数组。

`DetailWorkspace.vue` 是 Users、Upstreams、Releases、Enterprise Updates 和 Usage Request 的共享紧凑主从工作区：宽屏同时显示 collection/detail，窄屏进入详情后只显示详情并提供返回列表，不复制业务状态。实体内部再按稳定任务拆成少量 section/tab；例如 User 只暴露 Devices、Usage budgets、User usage 三个详情分区，危险动作收敛到 actions menu。`CursorPager.vue` + `useCursorPager.ts` 是主 collection 的有界上一页/下一页 primitive，只保留当前页和 cursor 历史，不把数千行持续挂在 DOM。`PagedEntityPicker.vue` 是潜在大集合的共享选择面，`api/entityPickerSources.ts` 提供 User、Upstream、Secret 的服务端 query/keyset cursor/selected-value resolve adapter；选择面独立呈现 loading/empty/error/load-more，不加载全部数据，也不要求操作员手填稳定 ID。

`UsersPage` 的二维码和复制按钮共用 `api/enrollment.ts` 生成的完整接入资料，平台 origin 取自 Admin 接入响应中的 `platformUrl`，不读取浏览器地址；裸注册码仅供显示排查。普通生成向 Admin API 提交空请求对象，由 Hub 应用协议默认时长，浏览器不维护第二份默认值。资料只在当前对话框内存中保存，关闭清理，不能放入 URL、日志或持久缓存。Client OpenAPI 定义原生消费的资料，`generated-client.ts` 仅提供其生成类型；Admin 仍只请求 Admin HTTP API。HTTP/HTTPS、域名/IP 均可使用。复制和 UUID 生成复用 Quasar 的 `copyToClipboard`、`uid`，支持普通 HTTP 上没有 Async Clipboard / randomUUID 的浏览器环境。

`console/src/` 当前以这些职责组织：

```text
api/          generated API + request infrastructure
boot/         app boot integration
components/   shared presentational/workflow primitives
composables/  browser/shared orchestration
layouts/      application shell
pages/        route-level pages
router/       routes + navigation registry
stores/       session/draft/activation/workflow state
i18n/         locale messages and localization
css/          thin MEASIX semantic styling
```

列表分两类，处理方式不同：

- **管理实体列表**（Users/Upstreams/Releases/EnterpriseUpdates）不能假定数据量小，用 `nextCursor` 驱动始终可见的上一页/下一页并替换当前页。**分页必须与搜索/高频筛选同时存在**：只给分页不给搜索时，操作员只能按创建顺序线性翻页找对象。凡 User、Upstream、Secret 等可能增长到数百或数千项且会被跨页面引用的实体，列表与 picker 都复用服务端 search/filter + keyset cursor；picker 还必须能按稳定 ID 单独解析当前值，确保当前引用不在首批结果时仍可读。
- **草稿内 collection**（Models/TTS/ASR/MCP/Assistants/Starters）以已完整加载的 Managed Draft 为 authority，在当前快照内做即时本地筛选；不得为它再造远端分页状态或第二份 store。
- **审计型列表**（用量请求、激活历史）是无界时间序列，必须同时具备显式时间窗、服务端 keyset 分页、当前页条数，以及有界高度的滚动容器。用量请求独立于汇总分析页签，按窗口列出并复用 `UsageRequestList.vue` 的有界分页；用户用量汇总同样不累积整库。激活历史在后端与契约层都有上界（`Release.activationHistory` 的 `maxItems`）。

**审计行必须自解释**：请求行直接显示用户与设备的显示名（由请求所属用户/设备表解析，不从用量行推断），使"这是谁的请求"无需先点开详情。`UsageRequestList.vue` 是 Usage 页与用户详情的共享实现，避免两处各写一份而漂移。

配置引用选择器不得用 `fetchAllPages` 预取全集。共享 `PagedEntityPicker` 只取首批结果、按输入重新查询并按 cursor 加载更多；编辑既有引用时通过 metadata GET 解析当前 ID。System 按概览、运行时交付、计量链路拆分诊断信息；未观测值显示未知而非零，也不为重复展示上游状态而遍历整个 Upstream collection，上游搜索、分页、配置状态和连接测试统一由 Upstreams 页面拥有。System 同时只读显示 Portal 的 STANDARD/CUSTOM/UNAVAILABLE 模式、Android 实际访问 URL 与 CUSTOM 上游 URL；选择模式属于 Hub 启动配置，不在 Admin 中复制一套可变部署配置。

`SettingsPage` 是部署级设置入口，不是任意 JSON/环境变量编辑器。当前可变项是会投影给 Client/Portal 的企业显示名称和 canonical public origin（界面名称为“企业地址”）：使用 `expectedUpdatedAt` 防止覆盖，并在同一事务记录 operator、名称与 origin 的 before/after 审计。修改 public origin 会立即影响新接入资料、Portal grant/同源校验和 Cookie Secure 策略，并只撤销现有 Portal 浏览器 session；Deployment、User、Device 和 Android Session 身份保持不变，客户端可直接改用同一 Deployment 的新地址。它不会配置 DNS、TLS、Caddy，因此界面使用一处简洁影响确认而不把编辑表单放进对话框。部署时区在当前版本固定，因为预算自然周期和企业日期边界依赖它；listen/storage/key、Relay、Portal 来源和 token/reconcile 参数仍属于启动或信任配置，只读展示并指向部署运维流程。

企业地址的页面预校验与 Hub 写边界遵循同一 Control Protocol host 语义：DNS host 在线协议中使用 ASCII/IDNA 形式，拒绝下划线、尾点和非法 label；浏览器可将用户输入的国际化域名转为 Punycode 后提交，Hub 不接受未编码 Unicode host。

Usage 顶层只常驻时间、用户和上游等高频条件，其余资源类型、状态、完整性、协议、额度健康与精确资源 ID 收进带生效数量的“更多筛选”；汇总、请求、核对、定价保持独立页签。用户额度卡片固定覆盖 MODEL/TTS/ASR/MCP/IMAGE_GENERATION 五类能力；图片仅允许 REQUESTS/REQUESTED_IMAGES。卡片把来源/模式/状态/修订/在途保留为紧凑摘要，只在存在累计用量或有限规则时展开对应内容，审计仍按需加载。一级 `Budget Templates` 在 Users 与 Resources 之间，与用户额度复用 `BudgetRuleEditor`；每个用户最多一个 live-linked 模板，用户显式能力覆盖优先，清除覆盖即回到模板/部署默认。模板身份和指派只在 Admin 展示，不投影到 Client/Portal/Snapshot/Runtime。定价只有本地规则相对已加载 revision 发生变化时才能保存。

当前实现已有 App Shell、route/navigation registry、PageHeader/status/health primitives、Users/BudgetTemplates/Resources/Upstreams/Releases/Usage/System/EnterpriseUpdates 等 route-level pages。

`ConfigurationSectionNav.vue` 只负责响应式分区导航；`ManagedExperienceEditor.vue` 负责 Assistant/Seed/Starter collection → selected settings。Draft 状态、引用删除、Validation、Preview/Publish 仍由既有 store/workflow owner 处理。`ResourcesPage.vue` 负责组合这些 owner，不创建平行状态或自由 JSON 编辑器。Shell route registry 以“Configuration & delivery / Operations & diagnostics”管理域分组；196px 桌面导航可手工折叠为 56px 图标栏，窄屏使用 overlay，同一按钮始终可恢复。S0.3 Enterprise Tools 及后续真实能力按域增加 route，不依赖序号切分，也不提前展示空导航。

## 3. 状态与 mutation 实现原则

前端只缓存 UI/workflow state，不创造 server authority：

- Draft/Candidate、Published Release、Active Runtime State 必须在 UI 上分开；
- Save Draft、Save Upstream Candidate、Apply、Publish 是不同 mutation；
- 202 Accepted 不等于 ACTIVE；Activation 必须可刷新恢复；
- Apply/Publish retry/recovery 在现有 ActivationStore 中按 kind + target/payload scope 重用同一 command Idempotency-Key；响应未确定时重试不清空 key，终态后的新命令另建 key。当前内存状态不提供跨浏览器重启的 pending-command journal；
- 409 stale revision 必须保留可恢复的 local edits，而不是静默覆盖；
- validation 以 Hub 返回为最终权威，前端可做即时提示但不能维护第二套业务规则；
- Snapshot Preview 必须消费 Hub canonical compiler 的 projection；Review 的变更计数由 Hub 将保存的 Draft 与最新 immutable Release 比较后返回，浏览器不得把当前 Draft 克隆成所谓发布基线。

## 4. 实现范围与后续验证

具体“必须做什么”只引用 architecture；当前实现与验证结果见 [当前状态](s0-execution-progress.md)。已有 S0.1 编辑/预览/发布/恢复代码和浏览器场景，不再将旧 C1/C2 执行单当作当前待办。代码存在仍不等于当前 candidate C6/C7 Green。

S0.2 Assistant/Memory Seed/Starter 由 Resources 内的 `ManagedExperienceEditor.vue` 编辑，复用唯一 DraftStore/generated DTO/Save/Validate/Preview/Publish 流程；Seed 支持空数组及作者顺序，Starter 绑定 Assistant，删除 Assistant 同时移除其 local Draft Starters。删除资源先检查 defaults、Assistant 和 binding 引用，不通过数组名猜测类型；Validation issue 携带 resourceKind/resourceId/field 并导航到对应分区。Review diff 与 canonical Preview 覆盖资源、Binding、Policy、Assistant 和 Starter；有未保存编辑时不运行 saved-Draft Preview/Validate。不存在第二套 API/store/schema。EnterpriseUpdatesPage 继续使用独立 Feed API；两者仍需按 ERX gate 证明真实 consumer 产品闭环。新增 Gateway profile 与运维状态不得借用现有页面截图声称已经实现。

不要通过增加第二套 schema、自由 JSON editor、客户端自定义 Provider body/header DSL 或隐藏失败状态来绕过这些要求。

Enterprise Update 的预览由 `useMarkdown.ts` 的独立 Marked parser 和 DOMPurify allowlist 渲染：仅保留合同文本子集，raw HTML 显示为文本，图片只保留替代文本，PLAIN 原样转义；不加载嵌入图片。单元回归覆盖 HTML/图片/脚本链接/代码块，浏览器 authoring 场景覆盖独立动态的创建、编辑、安全预览、发布、撤回与刷新持久化。

## 5. Browser E2E

`pnpm typecheck` 使用 `vue-tsc` 覆盖 `.ts` 与 Vue script/template（普通 tsc 不足）；Component/unit tests 使用 Vitest；真实产品 E2E 使用 `@playwright/test`。

当前 ownership：

```text
console/e2e/            browser actions/assertions
backend/test/system/    当前真实进程 system harness / deterministic Adapter / Test Client
scripts/               Node browser/candidate process + SPA orchestration
```

Browser E2E 必须使用 production `dist/spa` + real Control Hub + real Runtime Relay；禁止用 `page.route('/api/**')` mock Hub 来声明 T4.1 Green，也禁止直接写 DB 或调用 Relay internal API 完成被测业务对象。

日常界面回归保持小而明确：组件测试覆盖共享主从工作区与通用 picker 的状态契约，少量 E2E 覆盖最关键跨页流程；布局、信息层级、窄屏返回和真实 loading/empty/error 由 `device:real` 上的 production SPA 浏览器实操补充验证。不要为了覆盖每个视觉分支建立脆弱的大型 E2E 编排，也不能用单元测试替代真实浏览器审查。

完整 Playwright T4.1 不属于默认 GitHub Actions CI/CD；它是 S0.1 candidate 的显式 C6/C7 gate。默认 CI 的 frontend unit/component/typecheck/build Green 不能替代 browser product evidence。

## 6. 响应式与共享组件

响应式优先使用 Quasar breakpoint + CSS/Grid/Flex，只有交互模型真正变化时才分支；desktop/mobile 不维护两套业务逻辑。

共享 primitive 只在出现真实复用后抽取。Shell、PageHeader、status/health、operation state 等跨页能力可以共享；Activation、未来 AgentRun、AgentSpaceOperation 等领域对象不能为了 UI 方便合并成万能 operation model。

**间距与尺度**：控制台只有一套间距——元素之间的边距和栅格间距统一用 Quasar `xs`（4px），模板中不再出现 `sm`/`md`/`lg` 间距类。页面外边距由 `css/app.css` 的 `.admin-page`（4px）统一提供，页面不再使用 `q-page padding` 或局部覆盖。Shell 内容区为流式全宽，不设居中 max-width——管理台是数据界面而非阅读界面，居中列宽只会在宽屏下产生大片死白。卡片与横幅内部是 `8px 12px`（直接在卡片内渲染的元素用 `.card-inset` 与之对齐）。复杂详情、编辑、Review/Publish 和 Snapshot Preview 使用页面内工作区；对话框只保留短确认或局部输入，并统一使用 `.app-dialog` 尺度与内部滚动，不再出现内联尺寸或长内容弹窗。

**列表形状**：所有主列表（Users / Upstreams / Releases / EnterpriseUpdates / 用量请求）共用同一紧凑结构——工具行（search/filter）、行、列表底部始终可见的页码/本页条数/上一页/下一页。picker 才使用 load-more 累积已选择候选；主列表翻页替换当前页。分页控件属于它分页的 collection；打开详情后由 `DetailWorkspace` 保持宽屏主从、窄屏单列的同一选择状态。

## 7. 文档与完成声明

- 产品/UX 变化 → architecture Admin Product Requirements；
- required scenario 变化 → architecture Testing Spec；
- wire/state/security semantic 变化 → architecture Control Protocol，再同步 OpenAPI；
- Vue component/store/composable/依赖/命令 → 本仓库；
- 当前 checkpoint 状态 → `docs/s0-execution-progress.md`。

本文不声明 C1–C7 Green。完成状态必须来自当前 architecture baseline + exact implementation SHA 的 executable evidence。
