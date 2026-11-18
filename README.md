# MEASIX Platform Core

MEASIX Control Hub、Runtime Relay 与 Admin Console 的实现仓库。Enterprise Tool Gateway 是 S0.3 目标，当前没有对应 daemon 或 Gateway Control OpenAPI。

当前源码与固定的 `0.2.0-preview.22` 包需要分别判断，具体身份和证据边界见[实现与封版状态](docs/s0-execution-progress.md)。Android `0.0.20 / versionCode 20` 尚未正式发布，候选提交不等于正式发行。

## 阅读入口

先读[本地架构与文档治理](ARCHITECTURE.md)，再按同级架构仓库的[阶段阅读清单](../measix-architecture/docs/measix-stage-document-index.md)查找语义权威。产品、wire、状态、安全与阶段验收要求归架构仓库；本仓库维护源码、可执行合同、持久化、测试和操作事实。

## 文档导航

所有维护文档平铺在 `docs/`，按用途查阅；不另建计划、归档或重复索引目录。

| 用途 | 文档 |
| --- | --- |
| 开发与贡献 | [开发环境、启动与真机预设](docs/development.md) · [贡献流程](CONTRIBUTING.md) |
| 合同与消费 | [API、Snapshot、兼容与生成](docs/api-contracts.md) · [Android 消费参考](docs/android-platform-integration.md) |
| 功能实现 | [Admin Console](docs/admin-console-implementation.md) · [Direct MCP 工具治理](docs/direct-mcp-tool-governance.md) · [远程工作区](docs/remote-workspace-implementation.md) |
| 用量与成本 | [计量、额度和结算](docs/usage-budget.md) · [定价与费用分析](docs/pricing-cost.md) |
| 数据与运行 | [数据库迁移](docs/database-migrations.md) · [运行配置、健康与排障](docs/operations.md) |
| 验证与交付 | [测试、CI 与专项验证](docs/testing.md) · [发布与候选证据](docs/release.md) · [Spark Preview 部署、备份和恢复](docs/s02-preview-deployment.md) |
| 状态与历史证据 | [固定组合、源码增量与证据索引](docs/s0-execution-progress.md) |

实现完成后的方案将可持续参考的内容归入上述文档，过程记录由 Git 和原始验证产物保存。生成资料位于 `api/generated/`，只通过原生成入口更新。
