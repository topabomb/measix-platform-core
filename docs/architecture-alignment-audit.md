# Architecture / Core 对齐审查（2026-08-31）

## 1. 性质、范围与基线

本文件是一次源码与文档对照的**审查快照和修复计划**，不是新的产品合同、living status 或 Freeze 证明。当前阶段状态只维护在 [s0-execution-progress.md](s0-execution-progress.md)。本次只修正文档，下面的代码缺口尚未修复。

审查前工作树均干净，源码基线：

| 仓库 | 分支 | 审查前 HEAD |
| --- | --- | --- |
| measix-architecture | main | `d11ea643f2326db43c5a495a73490d09f2d966db` |
| measix-platform-core | agent/s0-platform-core | `bb7a689e20b3325cd6ade4bae0d477acd3cbd183` |

盘点覆盖 architecture 全部 31 份 tracked Markdown、core 全部 15 份既有 tracked Markdown，以及 core 的 459 个 tracked 文件清单。按以下边界阅读和交叉核对实现、OpenAPI、测试和工具；生成文件结合输入 schema、生成入口与 contract tests 核查，不声称逐行形式化验证所有生成代码。

| 审查域 | 主要材料/实现入口 |
| --- | --- |
| 权威和阶段 | 两仓库 README/AGENTS；架构文档指南、阶段索引、Roadmap、术语、Runtime/Experience 总图；core ARCHITECTURE/CONTRIBUTING/全部 docs |
| 产品与跨组件合同 | Foundation、Capability、Realm、Gateway、Android、Admin、Control Protocol；全部 Component/System/Adapter Testing Specs |
| HTTP/身份/持久化 | 四份 OpenAPI、fixtures、generated wire；Hub httpapi/identity/security/store/maintenance；Ent schema、SQL、启动入口 |
| 配置交付 | capability compiler/validation/diff、runtimecontrol publish/reconcile/security/operational；Relay control/auth/admission/routing/proxy |
| 计量与内容 | Relay metering/spool/sender；Hub usage/pricing/query、system、enterpriseupdate 和 Feed HTTP |
| Admin 与工具链 | Vue pages/stores/router/api、Vitest/Playwright；Go system harness；Node candidate/replay/qualification；Makefile、CI、historical manifest |

本轮未审计 Android/Portal 仓库实现、真实设备、生产网络或真实外部 Adapter；跨仓库 consumer 验收仍须分别执行。

## 2. 总体判断与文档决策

职责划分总体合理：Hub 是业务/配置事实权威；Relay 负责公开 Runtime admission/transport 和请求级计量；Gateway 是私有工具发现/执行组件；Admin 只通过 Hub Admin API 工作。Relay 不访问 Hub Ent/数据库、持久 spool/replay、canonical Snapshot Preview、generated Admin DTO 和独立只读 Feed 等方向应保留。

不能以这些组件存在或旧 manifest 为理由宣布当前 S0.1–S0.4 已闭环。文档有三种不同问题，必须分别处理：

1. **文档错误/越界**：陈旧文件引用、把目标目录写成现状、不可执行命令、无依据的 Windows/安全软件结论、历史审查标为“全部关闭”。本轮直接修正文档。
2. **架构语义缺口/冲突**：Gateway generation 映射、移除顺序、平台工具来源、工具面哈希、Feed 投影命名/一致性、计量条件字段。本轮在唯一 owning contract 内澄清，再同步引用与测试要求；新增的细化要求不倒推为历史版本已经承诺或已被验证。
3. **实现未满足合同**：会话/Discovery、Feed、当前 Admin、运维与证据链等。保留架构目标，在 core 明确缺口；不通过降级架构或双路径兼容来制造“对齐”。

进程监管与基本日志收集纳入 S0.3：以宿主原生 supervisor 管理 Hub/Relay/Gateway，各自故障隔离并提供聚合操作；不自建第四个管理服务，不引入集中日志平台。工具命名也明确：默认开启且受服务端 REQUIRED/DEFAULT_ON 控制的是 Gateway 的固定 `discover_tools`/`invoke_tool` 原子工具对；Direct Managed MCP 仍是 Assistant-bound 的独立路径，不把所有 MCP Server 全局强制注入。

## 3. 实现偏离与风险清单

P1 = 当前合同/验收可信度或生产运行的重要缺口；P2 = 完整性/工具体验缺口。这里不是自动执行代码修改的授权，也不是对线上事故的断言。

| ID / 级别 | 当前源码事实与证据 | 后续修复与验收方向 |
| --- | --- | --- |
| A01 / P1 | `identity/service.go` 的 enrollment 固定 refresh expiry 为 30 天；Refresh 只发新 access token，不旋转 credential/滚动 idle；`config/config.go` 的 RefreshTokenTTL 未传入 identity；`httpapi/handler.go` 响应缺 sessionIdleExpiresAt，enrollment 状态码和细分错误也未完全符合合同 | 按 Foundation/Control Protocol 完整实现会话生命周期、TTL 与 wire/error；并发 refresh、过期、撤销/禁用和重放测试，不能把无效配置写成可用 |
| A02 / P1 | Hub config/client view 需要并返回绝对 public/runtime URL，允许不同 origin；架构要求客户端发现的同源路径约束 | 统一 Discovery 可执行 schema、配置校验和 consumer 测试；不能只修改示例 URL，也不能以开发跨端口运行证明生产同源 |
| A03 / P1 | `capability/snapshot.go` 无条件生成 schemaVersion=2；保留 v1 golden helper 不等于当前服务仍交付 v1。memorySeed 被排序且 validation 要求至少一项；Starter 按 assistant/sortOrder 而非自身 stable ID 编译，同 sortOrder 无 tie-break | 按本轮明确的空 seed/作者顺序、canonical ID order 与 UI display order 分离规则修复 compiler/fixtures；预览、发布、Android export 同一权威；冻结版本哈希不可借机改写 |
| A04 / P1 | `enterpriseupdate/service.go` 用“当前最大行 revision + 1”，读取与写入无统一事务；Draft Create 也增加 Feed revision；Client handler 分开读取 published rows/revision | 用持久 revision 权威、并发安全事务和一致读视图实现 publish/withdraw/ETag；验证并发、撤回、空 Feed、Draft 不改变公开 Feed。不能用 MAX+1 当并发保证 |
| A05 / P1 | Feed handler 固定 UTC，Deployment 无 timezone 字段；日期上界使用 Add(24h)；OpenAPI/handler HTTP 使用 start_date/end_date、updateId，与 owning HTTP 合同不一致；ETag 只有 rev-N | 实现 Deployment 时区/自然日边界，统一 camelCase HTTP 与独立 snake_case 工具投影；查询/日期变化 ETag 和 DST 场景。查询敏感 ETag 等本轮新增细化要求应单列迁移影响 |
| A06 / P1 | `ResourcesPage.vue`/`ReleasesPage.vue` 没有 Managed Assistant/Memory Seed/Starter typed authoring；后端 wire/domain/preview 已有部分支持；EnterpriseUpdatesPage 使用 generated DTO，但其存在不能替代产品 E2E | 补 S0.2 Admin 实际创建→编辑→预览→发布消费闭环和 Feed 独立流程，引用 ERX 场景；不能用自由 JSON 编辑器或 API 脚本充当 UI 验收 |
| A07 / P1 | `runtime/handler.go` 与 `runtime/metering.go` 记录 RequestUsage 需要已解析 route/upstream，认证成功但 no-route 等拒绝未完整落账；Gateway target/source/conditional fields 尚无 OpenAPI/实现 | 先补已有安全主体的拒绝事实，再实现 targetKind/条件字段和 Gateway facts；不得伪造 upstreamId、Integration 或把旧请求的 applied generation 改记为新请求值 |
| A08 / P2 | 多个 Hub 列表接收 cursor 却只取 limit；Usage request listing 未形成完整 cursor/completeness 过滤闭环；System 主要暴露最近 ingest/lag，非完整 pending backlog | 逐 endpoint 对照 Admin OpenAPI/产品需求补分页/筛选/聚合语义与 UI 测试。内部 COMPLETE 映射为公开 EXACT 是已有转换，不应误判为枚举漂移 |
| A09 / P1 | `cmd/control-hub/main.go` 未设置 RuntimeOptions.AdminAssets；`adminstatic` 的 library/test 与 Node SPA server 存在，但无生产挂载参数/包装 | 明确实际静态托管 owner，生产构建验证 /admin deep-link、API 隔离与同源；不能宣称 Hub 二进制已经托管 Admin |
| A10 / P1 | Hub private 默认 :8081；main 用普通 HTTP；一枚 token 双向复用。`common/server` 超时 shutdown 无显式 Close/cancel；Relay fatal os.Exit 跳过 defer | 部署隔离/凭据范围、有限 drain/cancel/强制终止及 spool 保留必须实测；正常 readiness 与 degraded 分开，不把 ready 当作全部子系统 Green |
| A11 / P1 | `maintenance/database.go` 只检查初始 17 表，缺 Enterprise Update/current columns/history；revision.go 固定初始 revision；backup metadata 固定 s0-initial，且 orphan metadata 可被覆盖 | schema/history 检查与 System/backup 版本事实一致；新目标文件保护、完整 integrity/restore 测试、可执行恢复步骤。保留现有备份，不改写历史迁移 |
| A12 / P1 | `cmd/devmigrate/main.go` 自有 devmigrate_revisions，碰到 already exists 可继续；不是 Atlas revision 历史。存在两份迁移但检查工具不能完整证明升级结果 | 明确开发便利工具与发布 gate；迁移 hash/空库回放/旧版升级/失败重启全套验证，不把部分表存在当成迁移完整 |
| A13 / P1 | 无 Gateway binary、Gateway Control OpenAPI、v3 surface/catalog、三进程真实 harness、service units/collector；Hub/Relay JSON logging 缺统一 event/service/build/correlation | 按 S0.3 合同先 OpenAPI/fixture，再 Gateway/Hub/Relay/客户端 profile、Admin 和 supervisor/logging；未实现项不能用文档或假进程占位验收 |
| A14 / P1 | Make collect-artifacts/freeze-gate 的 cd 延续导致 write-meta 相对路径错误；collect-static-contract 续行含 @#、pipeline 错误传播不足，且 drift 未先 regenerate；make ci 不依赖 generate（CI static job 则显式生成） | 修复脚本工作目录/失败传播，干净 checkout 重生成后比较；对采集器自身做失败注入测试。脚本注释和文件存在不能当作执行证据 |
| A15 / P1 | freeze-manifest.mjs 固定 snapshotSchemaVersion=1；--validate 校验范围小于完整合同/输入 provenance；replay 不建立独立 clean checkout，部分输入 hash/dirty-state 未严格核对；real qualification 顶层 VERIFIED 不要求所有 profile 都 VERIFIED | 正确 pin schema/profile/source/build/fixture/qualification/artifact；逐 claimed profile 资格验证；明确 fresh-runtime replay 与 clean-source replay 区别。历史 manifest 不自动认证当前 v2/v3 |
| A16 / P2 | root start:relay 指向 Hub public 8080 的 private usage path；setup 再跑会重复 bootstrap；Playwright retries=0 配 trace=on-first-retry，且启用了忽略证书/禁用 sandbox 等参数 | 修复开发命令与安全测试配置；默认失败可取 trace；移除无实证的杀安全软件/固定 sleep 建议。当前测试不能证明生产 TLS/browser isolation |

源码表内未带目录的 Hub 文件相对于 `backend/internal/hub/`，Relay runtime 相对于 `backend/internal/relay/`；脚本相对于仓库根。接口具体字段和状态码以四份当前 OpenAPI及 HTTP handler 交叉核查，不以本文替代后续合同修改。

## 4. 架构侧本轮修正

- Hub 编译发布为唯一权威；目标包含 Gateway 时，每次 Publish 都先应用新 generation 映射，即使 catalog/surface 字节未变。移除 Gateway 先由 Relay 完成收紧，再异步清理旧 Gateway 状态；不可让离线 Gateway 阻止撤销入口。
- 明确 PLATFORM 与 DOWNSTREAM_MCP 的互斥来源，以及普通 Upstream 与 Gateway 的计量/路由条件字段。平台内置工具不伪造 Integration，Gateway 不伪造 Upstream。
- 固定两工具 surface，独立 JCS/SHA-256 规范与 fixture 要求，不修改冻结 Snapshot v1/v2 的哈希算法；参数遵循各自 published schema，拒绝不支持的 schema 能力不等于任意 schema 都强制禁止额外属性。
- 默认启用策略、用户可选状态和 interaction 捕获时点统一；HTTP Feed 与 Gateway 业务工具投影各自命名边界明确；补 Feed 事务、查询表示、时区测试。
- Admin S0.2 作者流程、阶段适用性、最终 manifest 引用与术语统一；物理目录/工具链版本/当前完成状态留在 core。

## 5. 推荐后续实现顺序（本轮未执行）

1. **恢复合同与证据基础可信度**：A01/A02/A07 的已有合同缺口、A11/A12 迁移/恢复、A14/A15 证据工具；先观察失败测试，再改实现。校验所有声称已完成的基础行为，不追认历史 Green。
2. **关闭 S0.2 服务端产品缺口**：A03–A06/A08，包含 canonical ordering、完整 Admin 作者流程、Feed 事务/时区/wire，以及 Portal 短期会话协议与 consumer 联调。后者须先在 owning Protocol/OpenAPI 固定完整交换/撤销/重放语义，当前 core 不能声明其完成。用同环境 ERX 场景出候选证据。
3. **实现 S0.3 可执行合同到生产闭环**：先 Gateway OpenAPI、v3/source/target/hash fixture，再 Hub 发布矩阵、Gateway catalog/toolRef/执行、Relay admission/usage、Admin/Test Client；同时完成 A09/A10/A13 的原生监管、日志和恢复。无需提前建设通用编排平台或把所有 Direct MCP 合并到 Gateway。
4. **真实消费者与版本化 Freeze**：基于 exact candidates 验证 Android/Portal，再运行对应 T4.2/T4.3/T4.4 与 final SYS。S0.3 可并行设计，但不能跳过其依赖的基础合同与 S0.2 验收输入。

每批同步同一个 owning contract、OpenAPI、fixtures、generated types、测试与本地操作文档；拒绝长期双读、双写、伪造来源和旁路入口。

## 6. 本轮执行证据与边界

- `backend/`: 首轮 `go test ./...` 退出码 0（部分缓存）；无缓存 `go test ./... -count=1` 退出码 1，`pkg/platformid` 与 `test/system/adapter` 的临时 test.exe 被 Windows 报告文件占用，未能启动，不是断言失败。随后独立串行 `go test -p 1 ./... -count=1` 退出码 0；原始失败保留，不推断文件占用的具体外部原因。以上均未包括 smoke/candidate build-tag 场景。
- `console/`: `pnpm typecheck`，退出码 0；`pnpm test --run`，14 文件、66 测试通过；jsdom 的 Window.scrollTo 提示不属于真实浏览器验收。
- 未执行 Atlas 全 gate、race 全 gate、production browser、real Adapter、Android/Portal、supervisor、Freeze/replay。Windows 当前 PATH 无 make；Make 采集器问题来自源码审查，不伪称实机复现。
- 仓库历史 manifest 声明 architecture=`cc60f8f540d309f2b73228094c8b9cd1b0b0a60f`、core=`a6075bc0afd78fa86d77e1a520f838c954c9adfa`、schema=1、required scenarios PASS。本轮未重放/完整验证其外部 artifact 链，故只称历史声明，不称“已验证有效的当前 Freeze”。该 JSON 保持不变。

本次盘点 47 份 Markdown（含新增本审查），检查本地 Markdown 链接目标和代码围栏，并执行两仓库 `git diff --check`。这些属于文档质量验证，不提升以上阶段状态。最终提交后的 doc commit 也不能冒充上述未修改源码基线的运行验收 SHA。
