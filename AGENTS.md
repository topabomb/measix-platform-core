# Core 仓库 Agent 协作约定

本仓库实现 MEASIX Control Hub、Runtime Relay 和 Admin Console；Enterprise Tool Gateway 属于 S0.3 目标。以实际源码和[状态页](docs/s0-execution-progress.md)区分已实现能力与规划。

## 任务执行与阅读入口

- 用户要求实现或修复时，连续完成范围内的修改、同步和相关验证；普通本地编辑与测试无需逐步确认。遇到影响产品语义、数据保全或交付范围的关键歧义时，先查权威资料；仍无法确定时再提出最小必要问题，并继续不受影响的工作。
- 先检查工作区现状，保留已有未提交修改；围绕本次目标修改，避免无关重构。使用中文沟通，说明关键发现和实际结果。
- 修改行为前阅读 [ARCHITECTURE.md](ARCHITECTURE.md)，再按[架构阶段索引](../measix-architecture/docs/measix-stage-document-index.md)定位相关权威。已读且未变化的内容无需在每次编辑前重复读取；纯文档措辞或格式修订只需检查相关段落和引用。
- 架构文档优先读取本地同级目录 `../measix-architecture`；缺失时先将 `git@github.com:topabomb/measix-architecture.git`（或 HTTPS 等价地址）克隆到该位置。产品、协议、状态、标识与安全语义由架构拥有，不能凭记忆或现有代码重新解释。
- 按任务读取实现参考：接口与生成材料见 [API 合同](docs/api-contracts.md)，前端见 [Admin 实现](docs/admin-console-implementation.md)，持久化见 [数据库迁移](docs/database-migrations.md)，验证见 [测试说明](docs/testing.md)，版本与发行见 [发布流程](docs/release.md)。其他资料从 [README](README.md)进入。

## 组件与数据边界

- **Relay** 不得导入 Hub 领域/Ent 包或访问 `hub.db`；保持供应商无关，供应商专属正文转换不放入 Relay。
- **Gateway** 不得导入 Hub 持久化领域/Ent 包或访问 `hub.db`，不得直接接受公网 Android 身份认证，也不得成为 Direct Managed MCP 的回退路径。
- **Admin Console** 只调用 Control Hub Admin API，不调用 Relay 内部 API。
- 协议类型从 OpenAPI 生成，不维护重复 DTO；跨组件规范样例（fixtures）只维护在 `api/fixtures/`，生成文件通过生成器更新。
- Secret 明文不得进入浏览器持久状态或日志。
- 保全持久草稿、不可变 Release 原始字节及 hash；SQL 迁移历史只追加。升级不得通过删库或重建已有数据实现。删除不受支持的原型时，保留仍受支持版本的恢复链与兼容测试。

## 版本对应与合同

跨端规则见[版本关联方案](../measix-architecture/docs/00-platform/measix-versioning-and-compatibility-plan.md)，具体字段、当前版本及 Android 调整步骤统一见[发布流程](docs/release.md)。

- 变更协议、状态、标识或安全语义时，先更新架构权威，再按影响同步 `OpenAPI → fixtures → 生成材料 → 测试 → 实现`；实现行为仍遵循下文 TDD。
- 支持架构明确的协议版本：当前新发布 Snapshot v5，并保留已发布 v4。Android 实现在其自身仓库完成，跨端兼容须有实际消费者验证。
- `api/protocol-baseline.json` 唯一维护 Core 的 `platformContractVersion`、`supportedPlatformContractVersions`、`coreBaselineVersion`；Client/Portal manifest 与基线副本自动生成。`baselineHash` 绑定 LF 归一化后的基线字节，不手改导出身份。
- Core 产品版本、Android `versionName/versionCode`、平台合同、Snapshot/Bridge/API 版本、SQL 迁移和业务 revision/generation 各有含义，不互相推算。根 npm package.version 只是工具元数据。当前号码及正式/候选状态读取发布文档和固定记录，本文件不维护第二份版本表。
- 未正式发布的候选修订不自动推进 Android 产品版本；历史源码或 Debug APK 不证明正式发行。支持集合只登记明确承诺，Android 自己拥有其集合，不能复制 Core 声明；保留 Snapshot v4 不等于已证明整个平台历史合同兼容。
- 对外直接说明“Android versionName（versionCode）已验证支持 Core 具体版本列表”；基准 Core 只表示最初对接版本。待验证、不兼容与已验证分清，不由合同号、材料 hash 或版本大小推算支持。
- 日常按“打包一次 → 固定对端原包测试 → 在已有发行记录写明双方版本、产物摘要和结果链接”执行。Core 独立升级不要求 Android 更新材料或重发 APK；后补结果引用原包，不改原包或重建已测产物。
- 默认打包无需 Android 记录；自定义证据格式、四类报告及证据打包只在主动采用时执行，不扩大为普通构建/发行的硬门槛。保留现有字段以兼容 Android 已开展的工作；生成/打包前拒绝占用输出，保全原包与失败证据。版本关联不增加在线握手、数据迁移或新的阶段门禁。

## 实现与验证

行为变更和缺陷修复执行 **Red → Green → Refactor**：先增加最小有效失败测试并确认失败原因，再实现修复，最后观察受影响检查的最新结果。纯文档修改不制造 Red 测试。

- 优先验证受影响行为；完成适用的仓库门禁后，仅在新增改动、失败或未解决疑点需要时扩大或重跑检查。不可通过弱化断言、跳过必验项或修改失败报告取得 Green。
- 合同、fixtures、生成器或迁移变化须同步对应产物。生成结束后再执行依赖这些文件的测试和差异检查，避免读写冲突。生成漂移校验与“重复生成一致”是不同结果，未提交生成改动不能宣称相对已提交版本的漂移检查已通过。
- Vue/浏览器常规依赖由 `console/package.json` 与 `console/pnpm-lock.yaml` 管理，无需架构白名单；新依赖须有实际用途，不重复 Quasar 或业务/协议权威，并通过类型检查、测试和构建。

常用入口按改动选择，完整门禁和环境要求见测试说明：

| 改动范围 | 工作目录 | 验证入口 |
| --- | --- | --- |
| Go 行为 | `backend/` | `go test <受影响包> -count=1`，按影响执行 `go vet` |
| Node 工具 | 仓库根目录 | `npm run test:tooling` |
| Admin 前端 | 仓库根目录 | `pnpm -C console typecheck`、`pnpm -C console test --run`、`pnpm -C console build` |
| 生成合同 | 仓库根目录 | 顺序执行 `node scripts/checks.mjs generate`、`node scripts/checks.mjs drift` |
| 版本元数据 | 仓库根目录 | `node scripts/verify-preview-contract.mjs --mode core`；真实跨端验证另按发布流程执行 |

## 文档与完成标准

- 同一事实只维护一处：架构拥有语义，实现文档拥有代码机制与命令，固定记录拥有发行身份和证据。完成的计划合并到现有参考文档，移除过时或重复内容；不为一次修复新增平行规格或长期操作流水账。
- 本文件保留长期有效的仓库约束、任务入口和验收要求；临时进展、具体版本表、测试次数及逐次修复记录放在各自已有归属处，避免重复和失效指令。
- 完成报告说明实际改动、架构影响、Red/Green 或适用验证结果，以及未执行项和剩余缺口。命令失败如实报告；材料缺失时不编造结果。
- 只有对应架构门禁实际执行且完整证据存在，才能声明 S0.1/S0.2/S0.3/S0.4 或最终 S0 Exit 完成。历史 Green 只作回归参考，manifest 的 PASS 字段、工具测试样例或浏览器替身不能替代当前固定源码、构建、合同及产物的完整关联与真实 Android 消费验证。
