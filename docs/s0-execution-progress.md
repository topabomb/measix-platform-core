# S0 Platform Core 当前实现状态

> 状态日期：2026-09-14。本文是唯一 living implementation/stage status。执行清单见 [A1–P3 计划](android-platform-contract-readiness-plan.md)。

## 当前唯一契约

MEASIX 从未发布。Snapshot v4、Bridge v3、local-read v2、Enrollment formatVersion 1 和当前 HTTP `/v1` 是唯一实现目标；这些编号用于识别当前 wire contract，不表示兼容旧实现。旧 Snapshot、四项策略 DTO、策略收养、字段补值、双读/双写、增量数据库升级和历史命令别名均不维护。Gateway/Snapshot v5 仍是后续阶段目标。

当前数据库只有 `backend/migrations/202609120001_current.sql` 一份完整初始化 SQL。空库初始化后以 SQL SHA-256 作为 `schemaIdentity`；非当前开发数据库或配置直接删除重建，不转换、回填或旁路打开。当前 schema 的原子初始化、重复核对、完整性、备份恢复和失败不污染测试继续保留。

## A1–P3 当前结果

| Owner | 已完成内容 |
| --- | --- |
| architecture | Admin 配置工作台、Android 风格的分区/详情编辑、响应式行为、严格字段与 Review 基线写入产品和测试合同；数据库合同统一为当前 schema identity；Android 接线只要求当前配置初始化，不要求迁移旧配置 |
| Core wire/backend | Admin/Client OpenAPI 要求 Assistants、Starters 和五项策略全部显式存在；资源字段执行当前最小约束。Bootstrap 直接创建完整当前 Draft。Hub Preview 将保存的 Draft 与最新 immutable Release 比较，返回资源、Binding、Policy、Assistant、Starter 的权威 diff |
| Core Admin | 一个配置工作台覆盖 Overview、Models、TTS、ASR、MCP、Assistants、Policy；桌面固定导航、窄屏选择器。Assistant 采用 collection → selected settings，包含 Basic、Instructions、Memory、Model & MCP、Starters。资源删除检查 defaults/Assistant/Binding 引用；Validation issue 可返回对应分区；脏 Draft 的刷新、离页和重新加载需要确认 |
| Core current-only cleanup | 删除旧的本地 diff 路径猜测、772 行重复 E2E、不可工作的 `freeze-gate` wrapper、旧 browser/schema 命令别名和迁移措辞；schema 工具只接受一份当前 SQL并拒绝增量历史 |
| Portal | 继续使用独立仓库和同一当前 Feed/Bridge 合同；远端与 bundled local 生产构建重新生成。Portal 不拥有 Managed 配置编辑，Core Admin 不复制 Portal 工作台 |

Android 集成导出包含 67 个内容文件及 manifest，当前 sourceHash 为 `d490580dd59ff21d32cfc430ec308ba902db6b49767765a9a89717fa5415d728`。导出校验拒绝错误协议版本、漏文件、多余文件、篡改和路径逃逸。

## 本轮验证

| 检查 | 结果 |
| --- | --- |
| Core backend | `go test ./... -count=1` 通过；`go vet ./...` 通过 |
| Core candidate systems | `go test -tags=candidate ./test/system/scenarios/ -count=1 -timeout 15m` 通过；真实 Hub/Relay/SQLite + deterministic Adapter |
| Core Admin | 17 个 Vitest 文件、85 项测试通过；`vue-tsc --noEmit` 与 Quasar production build 通过 |
| Core browser | `node scripts/e2e-harness.mjs` 通过 Admin authoring/publish、四类 runtime traffic、usage/system 和 topology security |
| Current schema | Atlas 对独立空库执行 apply/status：一份 SQL、37 statements、Pending 0；tooling 8 项测试通过 |
| Portal | 8 个 Vitest 文件、102 项测试和 2 项交付测试通过；format、remote/local production build 通过；本地 5 项与真实 Hub 6 项 Playwright 通过 |

Windows 当前 Go 环境未启用 CGO，`go test -race` 在测试启动前被 Go 拒绝；普通、系统和真实浏览器测试均已运行。没有执行 Android 编译/设备测试、真实供应商 qualification、独立 clean-source rebuild/replay 或最终 Freeze。

## Android 边界与下一步

Android 仓库仍由其维护方独立修改，本轮保持只读。其当前在途工作不能由 Core/Portal 的 Green 代替。Android 维护方下一步应消费上述 exact 导出，保持 Snapshot v4/五项策略/Bridge v3/local-read v2 解析一致，并完成真实 Platform source、远端 Relay 四 profile、428/刷新/退出和物理设备互操作证据。

S0.3 的 Enterprise Tool Gateway、Snapshot v5、真实生产 supervisor/package 和后续 User Sync 不在本批实现范围，也没有预建空模块或兼容入口。
