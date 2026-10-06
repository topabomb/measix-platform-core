# S0 实现与封版状态

> 状态日期：2026-09-24。本文只记录当前实现组合、封版边界和证据入口；历史修复过程由 Git 提交保存。产品与阶段验收条件以同级 `measix-architecture` 的 S0 合同和 Testing Spec 为准。

## S0.2 封版组合

项目将 `0.2.0-preview.22` 固定为 S0.2 Preview 封版组合。本机包内的 `.artifacts/releases/measix-core-0.2.0-preview.22-linux-arm64/release.json` 记录以下干净源码提交、四份 OpenAPI 摘要、生成产物及构建身份；归档 SHA-256 为 `ca7d480dccc6311923bcc024fd67ef7a42a82dca256567f25bb7b1212dfa2c28`。归档位置与证据边界见[发布说明](release.md)。

| 仓库 | 固定提交 |
| --- | --- |
| Architecture | `a52a630c75f5a857ebf72834d6770cae1cef14b8` |
| Platform Core | `632ade34a14e64b266d86da9e4b034e5005f0713` |
| Enterprise Portal | `7e3f5fb90955a2a44bea925e4f44e62267aab9bd` |
| Android | `1914e4a894979bb5a2fc892a019b9342ff60a28f` |

该包的可执行协议基线仍名为 `S0.2/v4-preview`，使用 Discovery API v1、Snapshot v4、Portal Bridge v3、Enrollment format v1。`preview.22` 是发布组合版本，不能当作协议版本号。后续源码提交不改变上述封版包；若替换封版内容，须重新构建并固定新的身份与证据。

## 已交付范围

Core 提供 Hub/Relay、Admin、同源 Portal 分发、Managed Release/Snapshot、企业接入与会话、Enterprise Updates，以及四种 LLM 协议、两种文生图、四种 TTS（其中 SYSTEM_TTS 在设备执行）、四种 ASR 和 Direct MCP 的当前合同。Relay 为十四个云端 profile 进行有界生产计量并持久投递；Hub 负责五类能力的额度准入、结算、核对、模板、Admin 分析及 Client/Portal 本人投影。动态用量与额度不进入 Snapshot，不推进 managedGeneration。独立 Portal 仓库提供标准企业工作台；Android 消费 Core 的当前平台协议和原生 Portal Bridge。

用户确认已在真机完成 S0.2 联调。此确认更新了此前文档中“Android 尚待接线/真机验收”的进度叙述；本仓库尚未收录逐场景设备运行记录，不能从 `release.json` 推导每条 `ERX-*` 为 PASS。补录证据时只索引实际日志、设备/安装包身份、浏览器结果和执行时间，不重写历史测试结果。S0.1 CAP 与 S0.2 ERX 的正式场景清单分别由对应 Architecture Testing Spec 拥有；现有 `scripts/freeze-manifest.mjs` 是 CAP 工具，不能作为 ERX 结果生产者。

## 当前可复核入口

- `api/protocol-baseline.json`、四份 `api/**/*.openapi.yaml`、`api/fixtures/` 与 `api/generated/android/` 是当前可执行合同与客户端资料。生成、校验和发布包构建入口见 [API 合同](api-contracts.md)与[发布说明](release.md)。
- Core Go、Admin/浏览器、静态合同及工具测试入口见 [测试说明](testing.md)；Portal 仓库的 `pnpm e2e:real` 使用隔离 Core/SQLite 和生产构建，Android Bridge 部分仍是浏览器替身。
- 真机联调所用的隔离本机服务见[真机预设](real-device-preset.md)；Spark 单机 Preview 的安装、备份和恢复见[部署手册](s02-preview-deployment.md)。已生成包不等于某现场当前安装版本，现场版本以运行进程和私有部署记录核对。
- 供应商实验是按日期固定的结果，见[真实供应商记录](real-supplier-integration.md)；协议解析覆盖、供应商资格和真机功能是不同证据，不能互相替代。

## 后续阶段边界

S0.3 的 Enterprise Tool Gateway、Snapshot v6、Gateway Control API、Catalog 与三 daemon 监管仍是后续工作。S0.4 的完整 Gateway/Android profile 及最终 S0 Exit 不由本次 S0.2 Preview 封版声明。S0.2 固定的现有功能可继续作为后续阶段回归基线。

## 当前 Starter 源码增量

本地源码在上述固定发布之后增加 Snapshot v5 Starter 开场编制，保全已发布 v4。方案、兼容和本次实际验证统一见 [Starter 开场快照](starter-opening-snapshots.md)。此增量未部署生产，不改变 preview.22 的固定身份与历史记录；Gateway 为后续 v6。

## 当前 Direct MCP 工具治理源码增量

本轮追加全部 11 个 Admin 管理页面的真实人工审查、1280/320px 浏览器回归与三个仓库全部 Git 变更复核。修复目录有界呈现、未核实/暂不可用/权限错误区分、空白公告提交和状态本地化；Admin 259 用例与完整 8 浏览器用例通过。范围、Red/Green 和验证限制见 [Admin 审查记录](admin-console-review-2026-10-06.md)，提交身份见各仓库 Git 历史及本地审查证据。

2026-10-06 未稳定 v5 升级为显式 ALL/ALLOWLIST：助手先绑定服务器，默认动态全部工具，需要编排时选非空工具白名单；服务器企业权限是共同上限。完整实施、UI 要求、Android 接线及实际验证见 [Direct MCP 工具治理](direct-mcp-tool-governance.md)。Core/Admin 已实现发现、审阅、许可、校验、Preview/Publish，并通过全量与真实生产 Admin 浏览器回归。Portal 传递类型/schema/摘要已同步，类型检查、95 用例、构建及 STANDARD/CUSTOM 实际浏览器通过。Android 本轮未改，新 consumer/device 验证待下阶段；跨仓库 gate 的 Android 差异仍保留。本次工作树候选不改变 preview.22 固定发布身份，不构成新的 S0.2/S0.3 或 final S0 Exit。
