# S0 Platform Core 当前实现状态

> 状态日期：2026-09-12。A1–P3 上游合同准备、实现修正与交付已完成；Android 真实平台 source、设备互操作、生产部署和 S0.2 Freeze 不属于本批完成声明。
> 本文件是唯一 living implementation/stage status。废弃原型审查文件与旧 Freeze manifest 已删除。执行细目见 [A1–P3 清单](android-platform-contract-readiness-plan.md)。

## 当前规则与实现

MEASIX 从未发布，当前协议和数据库结构是唯一版本。Snapshot 只支持 v4，五项 allowLocal* 均为必填 Boolean；Bridge v3、local-read v2；Gateway/v5 是后续目标。旧 v1/v2/v3 Snapshot、策略采用入口、历史 canonical 分支和数据库增量迁移不再维护。未知 optional HTTP 响应字段的读取规则仍是当前协议的行为，不等于支持旧版本。

| 仓库/编号 | 本批结果 |
| --- | --- |
| architecture A1–A4 | 统一版本与未发布前提；明确可信 origin/主体绑定、资源/Assistant/Seed/Starter/defaults 映射和排除字段；整理认证/刷新/待配置/428/退出时序；细化站点清理完成、失败重试、确认取消、媒体停止写入与旧文档隔离的验收责任。相关上位阶段合同与测试条目已同步清除旧 Snapshot 兼容要求 |
| core C1 | 当前 v4 可执行 schema 与 Go/TS/Android 导出；删除策略升级 UI/服务分支；修复无 Experience 内容时 compiler 输出 null 集合的问题，当前输出必需空数组；旧版本及缺失/null/错误类型策略被拒绝 |
| core C2/C4 | 真实 compiler 生成完整资源 profile、两名助手、有序/空 Seed、三项 Starter、允许/禁止策略；共享 schema、引用闭包、部署/generation/ETag 接收反例，以及四类 Runtime 请求示例。公开投影配方不含私有 Upstream/Binding，不伪装成可直接发布的运营草稿 |
| core C3 | 真实身份/SQLite/HTTP 回归覆盖待配置、已应用状态、Snapshot 200/304/404、退出后授权优先于 ETag；修复 generation=0 被报告为 Runtime 可用的问题。资料纠正 Bootstrap idle 到期时间、syncRequired/runtimeBlocked 和 refreshToken logout body |
| core C5/C6 | 完整独立导出含协议、案例、预期结果、架构与接线说明；原始字节 SHA-256、重复生成稳定、无兄弟仓库校验，以及篡改/缺失/多余文件/路径/版本拒绝测试；只维护本状态入口 |
| Portal P1–P3 | 保持现有工作台、同步/状态、日期/详情、手机能力体验；当前合同重新生成。补多块媒体组装、途中取消/失效与 logout 超时后恢复测试；真实 Hub 远端回归、双生产构建、交付校验完成，未改变 Bridge 协议或 Android 资源 |
| 当前数据库 | 8 份增量 SQL 收拢为 `202609120001_current.sql`。开发 helper 只初始化或核对当前版本，不执行增量升级；SQL 与初始化记录原子提交。当前备份/恢复、完整性、外键和重启检查保留 |

## 当前版本专项复审

- 删除无人调用的 Hub 原型 app.New：该入口忽略运行配置、直接声明就绪并暴露旧版本；正式启动继续使用 app.OpenRuntime。
- ManagedRelease 删除重复 snapshot_schema_version 列和生成代码，Snapshot JSON 保持当前协议版本的唯一来源；当前初始化 SQL 原地收敛，不新增迁移。
- 删除失效历史审查文件、旧 Freeze manifest、两项已无实现的升级场景要求及 qualification 环境变量旧别名。新增必需场景选择器与真实测试函数对应检查。
- 导出校验检查完整源文件清单，新增共享文件也必须重新导出。已用“新增源文件但旧导出仍通过”的失败测试复现，修复后通过。
- 架构删除遗留策略兼容与升级承诺；保留当前版本标识、generation、配置修订、认证幂等、当前备份恢复和供应商协议适配，这些不是旧版本兼容。
## 验证结果

| 检查 | 最新结果/证据 |
| --- | --- |
| 全量 Go backend | 564 个 test/subtest PASS，0 FAIL、0 SKIP；`.artifacts/version-review-backend.json`；删除未使用的原型入口及其测试后全量重跑通过 |
| 候选系统场景 | 42 个 test/subtest PASS，0 FAIL、0 SKIP；真实 Hub/Relay/SQLite + 确定性 Adapter，`.artifacts/version-review-candidate.json` |
| Go 静态 | go vet 全量通过，最终 gofmt 检查通过 |
| Admin | 前批 81 项单元测试、typecheck、production build 通过；本轮未改 Admin 行为，未重复运行；删除旧策略采用功能后当前五项策略可直接编辑 |
| 导出/工具 | 8 项 Node 测试通过；独立临时目录验证、全部 schema 依赖解析、重复导出稳定、错误版本/漏文件/篡改/额外文件/非法路径拒绝 |
| Portal | 102 项单元测试、2 项交付检查、typecheck、format、remote/local 双构建通过 |
| 真实 Portal 浏览器 | 6 项 PASS，零 retry/skip；5 项 local 生产页面场景与 1 项真实 Hub/SQLite 远端闭环；`.artifacts/browser.json`、`browser-meta.json`、`version-review-browser.log` 位于 Portal |
| Atlas | portable Atlas v1.2.0 实际 apply/status：1 SQL 文件、37 statements，Current Version 202609120001，Pending 0；`.artifacts/version-review-atlas.log` |
| 文档/交付 | 三仓库 diff whitespace 检查；Markdown 文件链接检查（排除演示占位 URL）；ZIP 文件清单与逐文件 SHA-256 校验 |

本轮没有运行 Android 编译/设备测试、真实供应商 qualification、独立 clean-source rebuild/replay 或最终 Freeze；以上通过不能代替这些门禁。race lane 未执行，普通 Go 并发轮换回归已执行。

## 当前交付

- [完整 Android 接线说明](android-platform-integration.md)：字段映射、请求/响应、状态与恢复、Runtime 四 profile、资料入口与分工。
- [独立合同导出](../api/generated/android/integration/README.md)：67 个内容文件，另有 manifest；sourceHash `78d689035a5c0b6d82b44081febac15734c32399fd528f3b50517c3cb1f07819`。`node verify.mjs --verify .` 可独立校验。
- [可转交压缩包](../../measix-enterprise-portal/.artifacts/android-integration-current-review-20260912-OYCoo5.zip)，解包目录 `android-integration-OYCoo5`。ZIP SHA-256 `64b15eaec4edc65889bc644cfa9a0d389054988acbc67e846960d13aa17a3961`。
- Portal local/remote 与浏览器证据 sourceHash 为 `811970a2bb5186437bb7a527a1d5f6ce718b639487a64ca2020027b89ff81695`；相对于 Android 已消费的 `b2953348…`，本包更新当前协议生成输入、交接说明、回归覆盖及打包来源校验，协议版本不变。

architecture 起点 `50bcf595196e8684895f5ed0626291985844308d`、core 起点 `95e8e815d3f9505d2eda08c15101f77f3e8f0380`，均叠加未提交工作树修订；Portal 尚无 commit。没有提交/推送/发布。构建内容摘要不是 exact commit 或 Freeze。Android 只读参考 HEAD `87ae6056f62913b1e5be3d0914df760fa5fdadfd`；其维护方有在途修改，本轮未写入该仓库。

## 待办与边界

1. Android 维护方消费本包，实现独立 Platform source、安全刷新 owner、完整候选映射与原子应用；接入真实 Relay 四 profile/428、远端 Portal 与设备生命周期，按本包身份关联证据。当前 LocalEnterpriseSource 仍明确返回 platform_enrollment_not_supported；本地改造完成不能等同真实平台接通。
2. 正式阶段门禁仍需真实四 profile Adapter qualification、独立源码重建重放与当前 exact candidate。CAP 工具现在使用当前 v4 的资源基线，不替代 S0.2 ERX。Gateway、User Sync、远程执行、S0.3/S0.4 和生产部署未在本批扩展。
3. 用户已删除旧本地数据库；本轮 node scripts/dev-setup.mjs 成功初始化当前 .data/hub.db，记录见 `.artifacts/version-review-setup.log`。不再存在旧库清理阻塞，无需再次删除当前库。
