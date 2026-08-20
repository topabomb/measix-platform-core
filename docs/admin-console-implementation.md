# Admin Console 实现规范

> Architecture authority：`topabomb/measix-architecture@6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b`  
> Product/UX：`measix-s0-admin-console-product-requirements.md`  
> Component architecture：`measix-s0-admin-console.md`  
> Required tests：`measix-s0-admin-console-testing-spec.md` + `measix-s0-capability-delivery-system-testing-spec.md`

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
- production 输出为 `dist/spa`，由 Control Hub/Ingress same-origin 静态托管；
- 浏览器只调用 Control Hub Admin API，不直连 Relay internal API；
- API DTO/type 来自 generated Admin OpenAPI；不维护平行手写 wire model；
- Secret plaintext 不进入 localStorage、持久 Pinia state、日志或测试 artifact。

Root repository 的 npm orchestration、实际开发命令与 system harness 生命周期由 `docs/development.md` 维护，不在本文重复。

## 2. 当前源码组织

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
css/          thin MEASIX semantic styling
```

当前实现已经有稳定 App Shell、route/navigation registry、PageHeader/status/health primitives、Users/Resources/Upstreams/Releases/Usage/System 等 route-level pages。

**当前 `ResourcesPage.vue` / `UpstreamsPage.vue` 仍承载了偏多领域编辑逻辑。** 这不是新的设计权威；应继续按 architecture 最新 S0.1 implementation decision 和 Admin Product/Testing requirements 收敛到清晰的 collection → selected editor/detail 与 feature-level workflow 结构。实际文件名可以随重构演进，但 page 不应长期成为所有领域状态/验证/编辑逻辑的容器。

## 3. 状态与 mutation 实现原则

前端只缓存 UI/workflow state，不创造 server authority：

- Draft/Candidate、Published Release、Active Runtime State 必须在 UI 上分开；
- Save Draft、Save Upstream Candidate、Apply、Publish 是不同 mutation；
- 202 Accepted 不等于 ACTIVE；Activation 必须可刷新恢复；
- Apply/Publish retry/recovery 重用同一 command Idempotency-Key；
- 409 stale revision 必须保留可恢复的 local edits，而不是静默覆盖；
- validation 以 Hub 返回为最终权威，前端可做即时提示但不能维护第二套业务规则；
- Snapshot Preview 必须消费 Hub canonical compiler 的 projection。

## 4. S0.1 当前实现重点

具体“必须做什么”只引用 architecture；当前 implementation 需要补齐的事实状态见 `docs/s0-execution-progress.md`。当前最重要的前端实现工作是：

```text
C1  existing Upstream candidate edit / Secret replace / recovery
C2  collection → selected editor/detail + Policy defaults + validation navigation
C3  Review → Client Snapshot Preview → Publish
C6  production browser E2E
```

不要通过增加第二套 schema、自由 JSON editor、客户端自定义 Provider body/header DSL 或隐藏失败状态来绕过这些要求。

## 5. Browser E2E

Component/unit tests 使用 Vitest；真实产品 E2E 使用 `@playwright/test`。

目标 ownership：

```text
console/e2e/            browser actions/assertions
backend/test/system/    当前真实进程 system harness / deterministic Adapter / Test Client
```

Browser E2E 必须使用 production `dist/spa` + real Control Hub + real Runtime Relay；禁止用 `page.route('/api/**')` mock Hub 来声明 T4.1 Green，也禁止直接写 DB 或调用 Relay internal API 完成被测业务对象。

完整 Playwright T4.1 不属于默认 GitHub Actions CI/CD；它是 S0.1 candidate 的显式 C6/C7 gate。默认 CI 的 frontend unit/component/typecheck/build Green 不能替代 browser product evidence。

## 6. 响应式与共享组件

响应式优先使用 Quasar breakpoint + CSS/Grid/Flex，只有交互模型真正变化时才分支；desktop/mobile 不维护两套业务逻辑。

共享 primitive 只在出现真实复用后抽取。Shell、PageHeader、status/health、operation state 等跨页能力可以共享；Activation、未来 AgentRun、AgentSpaceOperation 等领域对象不能为了 UI 方便合并成万能 operation model。

## 7. 文档与完成声明

- 产品/UX 变化 → architecture Admin Product Requirements；
- required scenario 变化 → architecture Testing Spec；
- wire/state/security semantic 变化 → architecture Control Protocol，再同步 OpenAPI；
- Vue component/store/composable/依赖/命令 → 本仓库；
- 当前 checkpoint 状态 → `docs/s0-execution-progress.md`。

本文不声明 C1–C7 Green。完成状态必须来自当前 architecture baseline + exact implementation SHA 的 executable evidence。
