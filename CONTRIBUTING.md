# Contributing

本仓库遵循 architecture-first 与 Red → Green → Refactor。开始工作前读取 [ARCHITECTURE.md](ARCHITECTURE.md)、同级架构仓库的[阶段阅读清单](../measix-architecture/docs/measix-stage-document-index.md)及相关实现参考。

## 变更归属

产品含义、组件边界、稳定 ID、wire/状态/错误、安全或阶段必验场景发生变化时，先更新架构权威。保持语义的实现、依赖选择、性能和界面内部调整留在 Core；发现语义歧义时先解决权威，不能从现有代码反推另一套合同。

- 合同调整遵循 [API 变更流程](docs/api-contracts.md#合同变更与独立演进)；OpenAPI、fixtures 和生成材料一起提交，生成文件不得手改。
- 数据调整遵循[数据库迁移](docs/database-migrations.md)：新增顺序 SQL migration，同步 Ent/generated code，验证空库、历史升级、幂等、原子失败及备份恢复。已应用 SQL、durable Draft 和历史 Release bytes/hash 不得重写或删除。
- Android 原生消费、Portal UI 和第三方服务实现由各自仓库维护；Core 导出不能替代其验证。

## 测试与 PR

行为调整和缺陷修复先观察最小有意义的 Red，再实现 Green，最后重构并执行受影响门禁。文档调整无需人工制造 Red。具体命令、真实边界和 GitHub-only 流程统一见[测试文档](docs/testing.md#tdd-与变更证据)。

开发使用短期分支和 PR，遵循已有 `.github/pull_request_template.md`。GitHub-only 行为变更先开 Draft PR，使 Red/Green 留在实际 Actions 记录中；当前 CI 只触发 PR to main / push to main。

PR 说明变更后的行为、架构/合同/数据库影响、实际 Red/Green、最新执行检查及剩余缺口。只报告已观察的结果；历史 Green、脚本中的 PASS 声明或 Core 单端测试不能证明当前阶段 Freeze 或 Android 设备兼容。不可通过删除断言、隐藏跳过或重复运行掩盖失败。

## 文档维护

按 [README 导航](README.md#文档导航)更新事实所属文档。架构不复制本地类名、DDL 或操作命令；Core 不重复定义产品语义。已完成计划和修复流水应移除，留下必要机制、可执行入口和历史证据定位。
