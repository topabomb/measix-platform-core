# MEASIX Architecture 文档结构与维护指南

> 状态：Documentation Authority  
> 文档职责：定义文档职责、权威关系和维护规则。阶段阅读清单只维护在 `measix-stage-document-index.md`。

## 1. 核心原则

1. **One fact, one authority**：同一事实只在一个文档中完整定义。
2. **从上到下细化**：战略/术语 → Phase/Stage contract → 产品/组件架构 → wire → testing → implementation repository。
3. **产品与实现分离**：architecture 固定“必须是什么、边界是什么、如何证明”；implementation repository 固定“代码如何组织和落地”。
4. **测试不创造语义**：Testing Spec 只证明已存在的产品/架构/协议要求。
5. **默认分支唯一权威**：正式架构只以 `measix-architecture@main` 为准。
6. **不建立平行 Implementation Spec 层**：具体 source tree、class/file、DDL、依赖、运行配置名和实现技巧归实现仓库；architecture 只保留跨组件长期稳定的技术决策与实现约束。

## 2. 文档职责

| 文档类型 | 职责 |
|---|---|
| Roadmap | 长期目标、Phase、major Stage |
| Terminology / Identifier Contract | canonical terminology、stable ID、版本字段角色 |
| Platform Domain Architecture | 跨 Phase 的独立领域对象、ownership、生命周期与子系统协作；不代替 Stage Contract |
| Phase Architecture | Phase 系统组成和 major Stage 边界 |
| Stage / sub-stage Contract | 交付范围、entry/exit、required profile、跨组件不变量 |
| Product / UX Requirements | 用户任务、信息架构、交互、可视化、产品 Exit |
| Implementation Decision | 必须跨仓库稳定的技术选择、工程原则、实施依赖和顺序；不维护具体代码结构 |
| Control Protocol | HTTP/wire/state/error/idempotency/security semantics |
| Component Architecture | 组件职责、状态/数据 ownership、依赖方向、持久化/恢复/安全不变量 |
| Component Testing Spec | 组件必须如何被证明；不规定测试文件目录或具体 runner 组织 |
| System Testing Spec | sub-stage/Stage 的跨组件 Gate 与 evidence |
| External Contract | 外部兼容边界 |
| Qualification Spec | 外部实现的验证流程与证据 |

只有真正存在独立用户工作流的组件才需要 Product / UX Requirements；不要机械扩张文档数量。

## 3. 权威层级

```text
Roadmap
→ Terminology & Identifier Contract
→ Platform Domain Architecture（适用时）
→ Phase Architecture
→ Stage / sub-stage Contract
→ Product / UX Requirements（适用时）
→ Implementation Decision（跨组件稳定决策）
→ Control Protocol
→ Component Architecture
→ System / Component Testing Specs
→ implementation repository concrete docs/code/tests
```

外部 Contract 对其边界拥有独立权威；Qualification 只验证它。

同一层级出现重复时，优先合并职责而不是继续增加“补充规格”。例如：

- Admin 的产品边界与组件边界可由一份 Product Requirements 承担；
- Android 某 sub-stage 已有完整 Integration Contract 时，不再并行维护一份重复的 Client Architecture；
- Component Architecture 可以说明“必须原子切换/必须持久化/必须 fail-closed”，但不能规定必须使用某个 class、helper、SQL DDL 或文件路径。

## 4. Architecture 与实现仓库边界

Architecture 可以固定：

- 产品任务、required capability 与 UX Exit；
- component/state/security boundary；
- cross-component wire semantics；
- persistence/atomicity/durability/recovery invariants；
- 对长期维护有跨仓库影响的技术方向；
- required test scenarios 与 release evidence。

实现仓库拥有：

- executable OpenAPI / generated artifacts；
- Go/Vue/Android 代码结构、class/file/package/module 名称；
- npm/Go/Gradle package 与 lockfile；
- DB schema、index、migration SQL 和具体 ORM mapping；
- concrete store/helper/component decomposition；
- 环境变量/flag 的具体命名（除非它本身是外部部署契约）；
- executable tests、CI、build、operations；
- 当前实现进展和 candidate SHA。

正常依赖选择和代码重构不需要先修改 architecture，除非它改变上述边界。

## 5. 文件与目录

- `docs/00-platform/`：平台级战略、canonical contracts 和跨 Phase 领域生命周期架构。
- `docs/10-runtime-foundation/`：Runtime Foundation Phase。
- `docs/10-runtime-foundation/s0/`：S0 及其 S0.1/S0.2/S0.3/S0.4 delivery sub-stage 文档。
- S0.1/S0.2/S0.3/S0.4 不建立独立目录，也不改变 S1/S2/S3 长期编号。
- 后续 stage/phase 只有出现正式内容时才创建目录。

命名：

- `*-contract-spec.md`：Stage/sub-stage contract；
- `*-product-requirements.md`：产品/UI/UX contract；
- `*-implementation-decision.md`：跨组件稳定实施决策；
- `*-system-testing-spec.md`：跨组件 Gate；
- `measix-s0-<component>.md`：仅在确有独立组件架构职责时使用；
- `*-testing-spec.md`：组件测试；
- `*-contract.md` / `*-qualification-spec.md`：外部兼容/验证。

不再创建 `*-implementation-spec.md` 作为 architecture 常规文档类型。需要长期稳定的实现约束应上移到对应 Contract / Component Architecture / Implementation Decision；具体实现细节下沉到实现仓库。

版本由 Git 管理，不用 `final/new/copy/v2(1)` 等文件名。

## 6. 维护规则

- 新术语/ID → Terminology Contract。
- 跨 Phase 领域对象/ownership/生命周期变化 → 对应 Platform Domain Architecture；长期价值和阶段仍同步 Roadmap。
- Phase/major Stage 变化 → Roadmap / Phase Architecture。
- S0/S0.1/S0.2/S0.3/S0.4 范围或 profile 变化 → 对应 Contract。
- Admin 用户任务/信息架构/UX/浏览器边界变化 → Admin Product Requirements。
- S0.2 ClientRealm、Managed Assistant/Memory Seed/Starter、企业动态或 Portal MVP 变化 → Enterprise Realm & Experience Contract / Portal Product Requirements。
- S0.3 Gateway surface/Catalog/Tool Integration/代理执行变化 → Enterprise Tool Gateway Contract / Component Architecture。
- 三 daemon 生产监管与安全日志的跨组件最低合同 → S0 Implementation Decision；具体 service units、配置、路径、保留策略和 runbook → `measix-platform-core/docs/operations.md`。
- Android S0.4 binding/managed-state/effective-runtime/full-profile/Gateway consumption 边界变化 → Android Integration Contract。
- wire/state/error/security 变化 → Control Protocol，再同步 executable OpenAPI/fixtures。
- Hub/Relay/Gateway 职责、ownership、durability/recovery 变化 → Component Architecture。
- required scenario 变化 → 对应 Testing Spec；若暴露语义缺口，先修上游 authority。
- 具体 package、目录、helper/store/component、DDL、依赖、CI 变化 → implementation repository。
- 阶段阅读清单变化 → 只修改 `measix-stage-document-index.md`，不要在 README/AGENTS/其他文档重复维护阅读顺序。

## 7. 防止文档再次碎片化

新增文档前必须回答：

1. 是否存在一个已经拥有该事实的 authority？若有，直接修改它。
2. 新内容是否具有独立读者、独立生命周期和独立验收责任？若没有，不新建文件。
3. 内容是否会随着代码重构频繁变化？若会，放实现仓库。
4. 是否只是把某上位文档重新展开一遍？若是，改为引用 + 本地 implication。
5. 删除该文档后是否仍能唯一确定产品/协议/测试要求？若能，通常不应独立存在。

## 8. 完整性

架构文档只说明“应满足什么、稳定边界是什么、如何证明”；实现是否完成，以对应实现仓库的 executable evidence 和该阶段 System Testing Gate 为准。
