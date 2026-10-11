# 实现状态与历史证据索引

文档核对日期：2026-10-11。本文区分当前源码、固定 Preview 包与历史实际验证；阶段要求由架构 Testing Spec 拥有。历史测试计数和修复流水留在 Git/原始产物，不滚动抄为当前 PASS。

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
- v5 Direct MCP 发现、ALL/ALLOWLIST、助手 binding 和 AUTO/REQUIRE_CONFIRMATION；见[工具治理](direct-mcp-tool-governance.md)。
- Agent Space 服务/用户生命周期、MCP binding、DAV 文件/文本编辑/预览与管理员资源摘要；见[工作区参考](remote-workspace-implementation.md)。
- append-only migrations、数据库保全启动、固定 build identity 的真机开发预设，以及 supported-version 响应扩展校验；见[迁移](database-migrations.md)、[开发](development.md)、[API](api-contracts.md)。

当前合同身份读取 `api/protocol-baseline.json`，真实支持读取实现和固定消费者构建；不由状态页赋值。源码存在不等于某现场已部署，运行版本须核对进程、active release 和私有部署记录。

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

10-02 专项包含未来版本拒绝时身份/历史/个人/退出、v4 覆盖安装、v4/v5/历史 v4切换、Starter 首发和实际工作区文件消费。分模块通过结果不等于一条全模块设备命令全部通过；系统 TTS/ADB 环境中断保留，真实麦克风、PRoot 镜像、OEM/正式签名安装未覆盖。之后工具许可或响应扩展修订仍须验证实际采用其材料的消费者，不能沿用此前整包结论。

## WebDAV 资源专项验证（2026-10-11）

本节记录 2026-10-11 首轮专项验证当时 `38c4271` 上尚未提交的 Core 实现与同级架构修订，未创建正式发行；下列证据和构建不自动覆盖后续审查修改。资源、权限、唯一文件交换指定、Activation、透明转发和基础统计已落地；Console 仍使用 Files。该轮独立子代理审查后修复上游 401 自动覆盖 key、XML 单 token 无界、未确认权限投影及 ACK 丢失后的控制恢复死锁，随后复审通过。

证据根为 `.artifacts/webdav-full-acceptance-20261011/`，其中凭据、DB 和私有账号清单不得公开分发：

| 范围 | 已执行结果与原始入口 |
| --- | --- |
| 实际前版升级 | `lan-001/evidence.json`、`run-003/evidence.json`；旧版 `38c4271` 可执行文件 SHA-256 `da6aac1a91b1d0f8b71ce547da3dd173bbfc91671c89234a09925e3ed21868d1`。旧版建立用户、空间、文件、凭据、Release、未发布草稿和显式撤销；迁移两次，40 张原表原列哈希及数量一致；另恢复备份由旧版 check 并读取 Files/Snapshot。并非每个旧业务表都有非空样本，历史用量由迁移回归另覆盖 |
| 真实 LAN Agent Space | `lan-001/transfer-1791687815009.json` 及其 `-verify-*.json`；按用户最终指示使用 536,870,912 字节视频。DAV、Files 上传及完整下载 SHA-256 相同，Range/If-Range/412/416、跨短 JWT 到期完成。原脚本误把合法 inline 文件名参数判失败，修正后重读 Files 全量摘要与预览通过；保留原失败报告 |
| 独立标准 DAV | `run-003/transfer-1791687825176.json` 及其 `-verify-*.json`；x/net/webdav v0.61.0，4,499,096,045 字节真实 MP4 的 DAV/Files 往返摘要一致，跨 4 GiB 位置 Range 正确。管理接口为替身，此结果不代表真实 LAN 的 4 GiB 验收 |
| 标准客户端和生命周期 | `lan-001/standard-client-*.json` 固定 webdav@5.11.0；`lifecycle-*.json` 验证关闭发布取消指定但 Files 可用、开启沿用同 ID、重设/撤销/恢复经 Activation、保留只读且恢复无需手工续 key |
| Console 实际交互 | `lan-001/console-{video,390,usage}.png` 与 `reviewed-build.json`；资源草稿开关/预览、权限编辑经轮询保留、待确认/生效/刷新、390px、键盘、Files 原生视频定位 99/100 秒并成功解码、关闭移除 video、DAV 512 MiB 统计详情无预算费用 |
| 自动检查 | 该轮 Go 全量 tests/vet、Console typecheck/44 文件 290 tests/build、81 项 tooling 通过；合同基线同步后 core verifier 通过，重复生成无变化。当时 `drift` 因预期生成修改尚未提交返回 1，这是历史检查结果，不代表后续提交的漂移状态 |
| 现场保全 | `lan-001/cleanup.json`：仅删除本轮两个专用测试空间/账号；原有 22 个账号身份、状态、凭据存在标记及卷关联一致。未读取原用户文件，也不据此声称逐文件内容校验 |

最终复审构建 Hub SHA-256 `2125b480342e29943f99a0bcf91340588fa8a86d96c3b58141477ac56df65438`、Relay `ad3285b546f858dfd68eab360dff6e798191a59bb41a125eb198a7254d920ea4`、SPA `7991dd1bdf71ab82b27a922121fe5694bb08280a8129e6dfae8f8785902ac840`。大文件证据使用其各自记录的前一轮构建；最后控制恢复修订由全量回归及上述真实生命周期覆盖，不能混写为一个固定发行资格。

仍未验证：Android/手机的新 DAV 消费、真实 IPv6 部署、真实上游 >4 GiB、同环境直连吞吐基准与全量 CPU/连接数、全部远端崩溃故障组合。真实上游镜像摘要未独立取得。LAN 最初超容量 PUT 返回 502（约 925 MiB），失败文件随后 HEAD 404；用户同意改用 512 MiB。该结果不证明历史间歇大文件问题根因已修复，也不表示 S0/S1 阶段完成。

## 保留的失败边界

- 隔离工作区大文件 PUT 曾在约 50 MiB/64 MiB 传输中返回 502/503 与 UNKNOWN，记录在 `.artifacts/workspace-files-final/`、`.artifacts/resources-review/`。新空间重跑通过不证明间歇传输根因已修复；不得自动重放未知写入，生产验收前继续核查。
- 旧隔离 Agent Space 容器首次文件访问还曾发生 VM 启动 SQLite 外键失败，见 `.artifacts/workspace-review-push-final/`、`.artifacts/workspace-review-approved/`。新卷/新容器通过不证明旧环境已修复，也不能据此认定与大文件中断同源。
- 历史 Starter 设备详情加载曾超时，独立测试环境复跑通过，未确认根因；原失败见 `.artifacts/starter-device-real/` 及当轮 core-android 留档。
- 历史供应商授权错误、静音录音失败、LEVEL_0 未知语义量分别判断；ready/ACTIVE 或 deterministic Adapter 不证明真实供应商恢复、实际录音成功或资格通过。

## 阶段与发布边界

Gateway daemon、Gateway Control、Snapshot v6、Catalog 与三 daemon 生产监督仍为 S0.3 工作；S0.4/最终 S0 Exit 与完整 S1 compute/storage 各需自身固定候选和 gate。CAP 工具不是 ERX 结果生产者，当前单端回归不替代 Portal/Android 实际消费验证。

复核入口统一在[测试文档](testing.md)；固定发行、总合同关联实施状态和 Android 待完成项在[发布文档](release.md)；Spark 操作在[部署手册](s02-preview-deployment.md)。本页不新增阶段完成声明。
