# 实现状态与历史证据索引

文档核对日期：2026-10-08。本文区分当前源码、固定 Preview 包与历史实际验证；阶段要求由架构 Testing Spec 拥有。历史测试计数和修复流水留在 Git/原始产物，不滚动抄为当前 PASS。

## 固定 S0.2 Preview 组合

项目固定 `0.2.0-preview.22` 为 S0.2 Preview 基线。归档 SHA-256 为 `ca7d480dccc6311923bcc024fd67ef7a42a82dca256567f25bb7b1212dfa2c28`；`release.json`、`SHA256SUMS` 原位置为 `.artifacts/releases/measix-core-0.2.0-preview.22-linux-arm64/`，分发时与归档一并保存。

| 仓库 | 固定提交 |
| --- | --- |
| Architecture | `a52a630c75f5a857ebf72834d6770cae1cef14b8` |
| Platform Core | `632ade34a14e64b266d86da9e4b034e5005f0713` |
| Enterprise Portal | `7e3f5fb90955a2a44bea925e4f44e62267aab9bd` |
| Android | `1914e4a894979bb5a2fc892a019b9342ff60a28f` |

该包基线为 `S0.2/v4-preview`，Discovery API v1、Snapshot v4、Bridge v3、Enrollment v1。`preview.22` 是 Core 组合版本，不是协议版本。后续源码、候选或设备 preset 不修改该包身份；替换内容需新构建与证据。

用户确认已做 S0.2 真机联调，但 Core 尚未收录该固定包的逐 ERX 场景、设备和 APK 运行记录。`release.json` 证明源码/协议/build/archive 身份，不能据此填 ERX PASS。Android `0.0.20 / versionCode 20` 尚未正式发布；上表 Android commit 和历史 Debug APK 都不占用其正式发行对应。

## 当前源码范围

Core 提供 Hub/Relay、Admin、同源 STANDARD/CUSTOM Portal 分发、身份/接入与会话、Managed Draft/Release、Enterprise Updates、LLM/文生图/TTS/ASR/Direct MCP、生产用量与用户额度/定价。动态用量和额度不进入 Snapshot、不推进 managedGeneration。

相对于上述固定包，当前源码还包括：

- Snapshot v5 的 Starter 固化开场与 published v4 读取/Republish 保全；规则见 [API 参考](api-contracts.md#snapshot-编译与历史数据)。
- 发布历史预览/手动/规则清理、上游实际引用诊断、独立编号与历史名称保全；见 [API 参考](api-contracts.md)和[迁移](database-migrations.md#发布历史迁移)。
- v5 Direct MCP 发现、ALL/ALLOWLIST、助手 binding 和 AUTO/REQUIRE_CONFIRMATION；见[工具治理](direct-mcp-tool-governance.md)。
- Agent Space 服务/用户生命周期、MCP binding、DAV 文件/文本编辑/预览与管理员资源摘要；见[工作区参考](remote-workspace-implementation.md)。
- append-only migrations、数据库保全启动、固定 build identity 的真机开发预设，以及 supported-version 响应扩展校验；见[迁移](database-migrations.md)、[开发](development.md)、[API](api-contracts.md)。

当前合同身份读取 `api/protocol-baseline.json`，真实支持读取实现和固定消费者构建；不由状态页赋值。源码存在不等于某现场已部署，运行版本须核对进程、active release 和私有部署记录。

## preview.23 固定候选与验证

2026-10-08 的发布历史清理/上游删除候选已在 Spark 完成内部升级与实际操作验证。以下身份绑定当次归档，后续源码或文档提交不改写原包：

| 项目 | 固定身份 |
| --- | --- |
| Core 版本 / 包分类 | `0.2.0-preview.23` / `UNVERIFIED_CANDIDATE` |
| Architecture / Core / Portal 提交 | `5fea0fe8e4a8e6f867cac9a1c013da18dda69c83` / `027f3a5e5931efc6c0e38b17bdf045d43ccfd71b` / `824b138d11dcc20c8b58135bdbeafba625e28ce3` |
| 归档 SHA-256 | `57f4907f53933fc0930c304323eb69867f88ec567b7a65c168db2b4b3a1e4847` |
| buildHash | `sha256:9fd443f23bda6b6b2ffd0cac3212366fd430b9fa3c9b5466d820c9cdc3ec59af` |
| baselineHash | `sha256:2d1226ee463e35b3c69b1e5b4ecf36a71be067d0cbc7b5456e2c3057c5030089` |
| Hub schema / migration identity | `5` / `sha256:2753bb2a075a831a6f9eeddc8a2bfff5b193b633a8c24fbeb572abe5883744b3` |

归档为 `.artifacts/releases/measix-core-0.2.0-preview.23-linux-arm64-candidate.tar.gz`；包内 `release.json` 与 `SHA256SUMS` 固定原始构建。具体站点地址、进程、备份路径与权限只维护在私有部署记录。

该候选完成全量 Go 测试/vet、Console 类型检查/单元回归/生产构建、tooling、生成一致性/drift 与 Core 合同材料检查。真实历史库副本及现场迁移验证原数据、自增高水位、完整性和其他外键保全；线上实际清理指定终态历史版本并独立删除无引用连接，保留版本原 content/Snapshot/hash、差异摘要与 1,320 条原用量及记账事实不变，自动规则保持关闭。并发新增发布及设备向该发布推进的 Applied 报告单独核对，不误记为清理修改。

上述结论只对应本固定构建和此次边界；未新增固定 APK、OEM/生产签名或完整 ERX/CAP 验收。原始结果定位见下表，现场恢复须同时保留匹配 schema 的旧程序和已验证完整备份。

## 历史证据入口

以下路径为历史材料定位，部分位于 Git 忽略目录或工作区外；换机/缺失时从原归档取得，不从摘要重造结果。每份结果只适用于其所记输入与运行，不认证当前 head。

| 范围 | 原始入口 | 可支持的历史结论与边界 |
| --- | --- | --- |
| 2026-09-18 真实供应商资格实验 | `.artifacts/real-adapter-qualification.json` | 模型/TTS/MCP 有转发记录，但 Adapter 构建身份未获验证，报告 FAILED；缺少 ASR/文生图完整资格，不能用于正式晋级 |
| Starter 编制/浏览器/原生闭环 | `.artifacts/starter-delivery-review/`、Android `build/reports/starter-delivery-review/` | 指定候选的开场、首发、Room 和重开；真实供应商与持续有声输入未因此获验收 |
| 2026-09-27 旧消费者兼容 | `.artifacts/compat-old-android-20260927/`；Android `tools/compatibility/` | 固定旧 Debug APK 的实际 v4 → v5/历史 v4恢复；不含 OEM 真机或生产签名/R8 |
| 2026-09-29 工作区文件联调 | `.artifacts/workspace-review-clean/{evidence,source-inputs}.json`、`.artifacts/workspace-file-edit-review/` | 隔离真实 MCP/DAV、64 MiB、条件编辑/取消、双用户与浏览器；不证明完整 S1 或全部远端故障 |
| 2026-09-30 资源摘要/配置简化 | `.artifacts/resources-review-final/`、`.artifacts/workspace-config-simplify/` | evidence、browser-evidence、source-inputs 固定同轮构建/合同与镜像；后续提交须重新验证 |
| 2026-10-02 v5 跨端专项 | `%USERPROFILE%\Documents\MeasixValidation\2026-10-02-v5` | `core/v5-protocol`、`core/starter-e2e`、`android/v5-protocol` 与 `binaries` 保存源码/build/APK/设备与失败；候选为 Architecture 07fdf4e、Core 34decd0、Portal 6b96d10、Android 3d47418de |
| 2026-10-04 DAV 未知操作恢复 | `.data/device-real/recovery-dav-20261004/` | 在线备份后经正式 API 核实断开、改配置、恢复原空间和显式 DAV；本机开发恢复，非生产或手机直连 DAV |
| 2026-10-08 发布历史/上游删除 | `.artifacts/release-cleanup-20261008/deployment-final.json`、`postconditions.json` 及浏览器截图 | 上述 preview.23 固定包的真实迁移、指定清理/删除、历史数据与其他服务保全；原始站点材料私有，不代替完整跨端门禁 |

10-02 专项包含未来版本拒绝时身份/历史/个人/退出、v4 覆盖安装、v4/v5/历史 v4切换、Starter 首发和实际工作区文件消费。分模块通过结果不等于一条全模块设备命令全部通过；系统 TTS/ADB 环境中断保留，真实麦克风、PRoot 镜像、OEM/正式签名安装未覆盖。之后工具许可或响应扩展修订仍须验证实际采用其材料的消费者，不能沿用此前整包结论。

## 保留的失败边界

- 隔离工作区大文件 PUT 曾在约 50 MiB/64 MiB 传输中返回 502/503 与 UNKNOWN，记录在 `.artifacts/workspace-files-final/`、`.artifacts/resources-review/`。新空间重跑通过不证明间歇传输根因已修复；不得自动重放未知写入，生产验收前继续核查。
- 旧隔离 Agent Space 容器首次文件访问还曾发生 VM 启动 SQLite 外键失败，见 `.artifacts/workspace-review-push-final/`、`.artifacts/workspace-review-approved/`。新卷/新容器通过不证明旧环境已修复，也不能据此认定与大文件中断同源。
- 历史 Starter 设备详情加载曾超时，独立测试环境复跑通过，未确认根因；原失败见 `.artifacts/starter-device-real/` 及当轮 core-android 留档。
- 历史供应商授权错误、静音录音失败、LEVEL_0 未知语义量分别判断；ready/ACTIVE 或 deterministic Adapter 不证明真实供应商恢复、实际录音成功或资格通过。

## 阶段与发布边界

Gateway daemon、Gateway Control、Snapshot v6、Catalog 与三 daemon 生产监督仍为 S0.3 工作；S0.4/最终 S0 Exit 与完整 S1 compute/storage 各需自身固定候选和 gate。CAP 工具不是 ERX 结果生产者，当前单端回归不替代 Portal/Android 实际消费验证。

复核入口统一在[测试文档](testing.md)；固定发行、总合同关联实施状态和 Android 待完成项在[发布文档](release.md)；Spark 操作在[部署手册](s02-preview-deployment.md)。本页不新增阶段完成声明。
