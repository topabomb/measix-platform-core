# 实现状态与历史证据索引

文档核对日期：2026-10-14。本文区分当前源码、固定 Preview 包与历史实际验证；阶段要求由架构 Testing Spec 拥有。历史测试计数和修复流水留在 Git/原始产物，不滚动抄为当前 PASS。

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

原包现保存在 `.artifacts/releases/measix-core-0.2.0-preview.23-linux-arm64-candidate-027f3a5.tar.gz`；包内 `release.json` 与 `SHA256SUMS` 固定原始构建。具体站点地址、进程、备份路径与权限只维护在私有部署记录。

该候选完成全量 Go 测试/vet、Console 类型检查/单元回归/生产构建、tooling、生成一致性/drift 与 Core 合同材料检查。真实历史库副本及现场迁移验证原数据、自增高水位、完整性和其他外键保全；线上实际清理指定终态历史版本并独立删除无引用连接，保留版本原 content/Snapshot/hash、差异摘要与 1,320 条原用量及记账事实不变，自动规则保持关闭。并发新增发布及设备向该发布推进的 Applied 报告单独核对，不误记为清理修改。

上述结论只对应本固定构建和此次边界；未新增固定 APK、OEM/生产签名或完整 ERX/CAP 验收。原始结果定位见下表，现场恢复须同时保留匹配 schema 的旧程序和已验证完整备份。

### 同版本 MCP 修订

目录发现及助手强制项修订于 2026-10-08 在 Spark 完成部署。版本仍为 `0.2.0-preview.23`，合同 2、Snapshot v5/v4 支持集合及 SQL schema 5 不变；新包与上述原包分别保全。

| 项目 | 固定身份 |
| --- | --- |
| Architecture / Core / Portal 提交 | `f6b028de261e33f52fb3edc275e13f9c6621fd3f` / `a1444056f5336b5739a1015e5c0f492705446e0e` / `4dc23b3dd2d6cd44d4668bbb60b0f914bea54940` |
| 归档 SHA-256 | `09f333bb49e789ae3181f0a72f87b917d58e5865138e833e9837f78f4edbfb61` |
| buildHash / baselineHash | `sha256:cb8bcd87f85bcd34f892c57e527d9f46a662e18fc226aa691f4d291ebfdd6855` / `sha256:389b429ae51eb8d8cfbbf927c028ad298b0e1471edb668b86e28cb778224804b` |

原归档保存在 `.artifacts/releases/measix-core-0.2.0-preview.23-linux-arm64-candidate-a144405.tar.gz`，分类仍是 `UNVERIFIED_CANDIDATE`。完整 Go 测试/vet、Console 300 项及 Portal 101 项测试、类型检查/构建、生成漂移与 11 项真实 Hub/Relay/生产 SPA 浏览器用例通过。新增用例覆盖零/一/多发现连接、版本诊断、空强制集合和无人绑定的 enabled MCP 发布/投影；原 v4 bytes/hash 与恢复回归通过。

现场包与全部文件摘要匹配，Hub/Relay 实际进程指向该构建；迁移 5→5、应用 0 项，完整性及外键检查通过。停服前草稿、4 份发布原始正文/快照、所有表记录数、配置/密钥和原 active generation 30 保全，9 个无关 PM2 进程 PID 不变。公共 HTTPS 的 Admin/Portal/Discovery/readiness 与内部路径隔离通过；生产 Admin 使用真实工作区连接成功发现 5 个工具，仅更新目录证据，未发布配置。证据在 `.artifacts/mcp-delivery-20261008/` 及私有现场记录。

`mcpBindings` 的强制项与用户额外选择规则见[工具治理与 Android 接线](direct-mcp-tool-governance.md)。Android 解析、偏好写入、选择投影和执行复验仍待实施，未运行该新组合的原生 consumer/device 或真实 tools/call；Core 导出和此次部署不宣称 Android 已支持新语义。

### 同版本定价与核对修订

2026-10-08 定价管理与批量核对修订已在 Spark 部署，版本仍为 `0.2.0-preview.23`。资源/上游可按名称选择，计量随资源类型约束；列表显示范围、价格与本地生效区间。待核对支持当前页多选、统一原因、执行前预览及逐项结果；请求数事实保留在详情，列表移除重复的单次请求徽标。

| 项目 | 固定身份 |
| --- | --- |
| Architecture / Core / Portal 提交 | `8da4d4c05018d581c08663c2ec9265588e53246c` / `0bd2cf86a8bd899c353565ea136b0a594d362eaf` / `4dc23b3dd2d6cd44d4668bbb60b0f914bea54940` |
| 归档 SHA-256 | `6ee41a147df01dd32be89a1bd660f0644168bd46607b302d371923ec8b0ef27f` |
| buildHash / baselineHash | `sha256:d8aad71b7a0fccca3a2d4f7e1f78c3baa377744ce2fbcca1b5c9930e4c07a429` / `sha256:389b429ae51eb8d8cfbbf927c028ad298b0e1471edb668b86e28cb778224804b` |

归档为 `.artifacts/releases/measix-core-0.2.0-preview.23-linux-arm64-candidate.tar.gz`，分类仍为 `UNVERIFIED_CANDIDATE`；原 MCP 包独立保全。此次修正 ASR 对全局字符价格的错误缺价判定、定价修订并发校验，以及失败写入后刷新仍保留旧表单导致覆盖的风险。规则优先级、完成时生效区间、输入/缓存拆分及未知成本保留经回归核对。

完整 Go 测试、usage vet、Console 类型检查、307 项单元测试与生产构建通过；11 项真实 Hub/Relay 和生产 SPA 浏览器用例全部通过。打包后的 Admin 静态文件与浏览器测试构建逐文件相同。线上 Hub/Relay 进程指向本固定构建，readiness 为 200；迁移 5→5、应用 0 项，schema/migration identity 不变。停服最终备份验证并离机保存；正式迁移前后所有 Hub 表记录数、定价/发布/草稿/状态事实与配置/密钥一致，完整性及外键检查通过；9 个无关 PM2 进程 PID/状态不变。

公共 HTTPS 的 Discovery、Admin、Portal 和 readiness 返回 200，内部路径返回 404，Admin index 与原包摘要相同。生产页面实际验证名称搜索、TTS 计量联动及批量核对预览；未保存价格或执行生产核对。原始证据在 `.artifacts/pricing-release-*.log`、`.artifacts/pricing-delivery-20261008/` 和私有部署记录。本次未运行新的 Android/真实供应商账单验收，不扩展原消费兼容声明。

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

### 提交前全量审查与界面复核

2026-10-11 再开后端、Console、合同/文档三个独立子代理审查全部 Git 改动，并合入远端 Core `e5e17c9` 与架构 `8da4d4c` 的历史清理、MCP 和计价优化；DAV 迁移顺延为 `000006`，保留既有 `000004/000005` 原字节。已完成方案内容归并到工作区实现、API、测试及用量文档，移除临时 plan。

- 新增测试先复现并修复：五个控制意图入口编译后权限变化导致未决 Activation 无法恢复、100/103 被误记为最终 HTTP 状态、独立 DAV 修订变化后旧 Token/迟到查询未失效、旧目录请求失败/取消污染当前文件列表。合并时补齐公共资源名称解析的 DAV 分支；历史迁移回归按全部原列逐字比较，并额外确认新增 DAV 统计列不会重新分类旧用量。
- Go 全量 `go test ./... -count=1` 与 `go vet ./...`、Console 46 文件 316 tests/typecheck/build、81 项 tooling 通过。最后工具栏样式调整后另跑受影响 15 tests/typecheck/build 通过。生成材料已在合并后重新生成；提交后执行生成漂移与合同基线校验，结果保留于下述证据目录。
- `.artifacts/webdav-console-review-20261011/run-001/`：用远端 `e5e17c9` 的真实旧可执行文件建库，重复迁移后 41 张原表原列保全；恢复旧备份由旧可执行文件读取 Files/Snapshot。标准 webdav@5.11.0 客户端及资源开关、签发、撤销、断开恢复的 Activation 生命周期通过。此轮上游为独立标准 DAV + 管理替身。
- `ui-review.json` 与 `console-{desktop,390}-final.png` 固定最终 SPA `6b53bd9ac56befed1dc2cc78777cae209d6cb4fd8d76e43b83c2d89c33b0e2c2`。实际浏览器核对全宽文件区、折叠配置/维护/权限/容量、新建中文 Markdown、当前目录筛选/清除、键盘预览及 390px 无横向溢出；Console 仍只使用 Files。`console-preview.png` 为最后工具栏压紧前的预览证据。

本次提交前复核未重跑真实 LAN 大文件、Android 或手机；前节 512 MiB LAN 与 >4 GiB 独立 DAV 证据仍对应各自固定构建，不能改称最终提交的大文件验收。

### 同步后正确性复核与真实本机联调

2026-10-11 本机 Core 从 `e5e17c9` 快进到 `2d1e350`，架构从 `8da4d4c` 快进到 `4e1d270` 后再次复核。继续遵循独立 DAV 资源、按用户授权与确切 Activation ACK、Files 独立管理、可选 MCP 和仅统计的边界；未修改合同、生成材料或迁移，也未创建正式发行。

- Relay 先于转发拒绝根目录删除/移动/覆盖、根目录 Destination、重复斜线别名和 `.agent-space*` 保留路径，保留根目录读取、属性和锁的标准行为。Console 降权须确认；网络或服务端失败后锁定原权限操作，以原请求正文及幂等键核实，轮询修订变化不会制造新操作。两项行为修复均先复现失败测试再修复；部署手册同步为 control v3、迁移至 006 和持久授权意图的恢复规则。
- 当前 Go 全量 tests/vet、Console 46 文件 316 tests/typecheck/build、81 项 tooling、生成漂移和 Core 合同材料校验通过。真实浏览器验证文本预览、降权确认及 390px 无横向溢出。
- 隔离本机实例连接用户提供的真实 Agent Space；原 `local` 账号仅作管理查询、MCP 握手/目录及 DAV 只读探测。另建两名专用测试用户/空间，验证发布 DAV 与可选 MCP、字节往返/Range、跨用户隔离、只读与完整切换、Files 独立、MCP 读取及无预算/定价的请求统计。标准 webdav@5.11.0 客户端与关闭发布、凭据重设、显式撤销、断开恢复生命周期均通过；实例和测试空间保留以供继续验证。
- 真实 MP4 的 4,400,476,038 字节及容量内 540,079,753 字节上传均返回 502。前者还超过该测试空间的 1 GiB 容量；直连同一 Agent Space、绕过 Hub/Relay 的上传也返回 502，随后 HEAD 503，日志为 `guest tcp connect: agent client error: client closed`。不能声明大文件验收或上游故障已修复；未自动重放未知写入，也未修改独立 Agent Space 源码或重启其容器。

本轮证据为 `.data/workspace-review-20261011/review-result.json`、`compiled-changes.diff`、生命周期/标准客户端/失败传输报告及浏览器截图，记录当前二进制、SPA 和上游镜像摘要。该目录也含私有环境与凭据，不可整目录公开分发。未验证物理第二台 LAN 设备、Android/手机新 DAV 消费、IPv6 部署或生产发布；本轮结果不覆盖上节历史大文件证据或 S0/S1 阶段门禁。

## 服务重建后的连接修复与本机更新（2026-10-12）

Core `2d1e350`、Architecture `4e1d270` 均与本轮最新 fetch 一致；其上为尚未提交的修复。机制统一见[工作区实现参考](remote-workspace-implementation.md#独立服务已重建后的修复)，不将全新空间描述成文件恢复。保留既有按钮对齐和原生 details 的 box-sizing 修正；服务表单、连接诊断、权限及容量仍按需展开，不增加常驻信息面板。

Go 全量 tests/vet、相关工作区/HTTP/控制包的最新回归、Console 全量测试/typecheck/生产构建及 81 项 tooling 通过。观察到地址保存、修订 CAS、ACK 前禁止创建、未知创建不重发、批次内显式接管、前端未知请求固定正文/幂等键和恢复入口等 Red，再修复到 Green。Admin 新合同、fixtures、Go/TS 及基线身份导出同步；重复生成一致，core 材料校验通过。相对 HEAD 的 drift 因预期生成改动未提交返回 1，不能记为通过。

隔离验证使用生产 SPA、真实 Hub/Relay 和独立确定性管理/DAV 替身：页面确认空空间边界后一次重新开通三个空间，原 Activation ACK 完成，原用户、草稿、发布字节和旧 SecretVersion 逐行保全；完整权限 PUT/GET 往返、只读 PROPFIND 207/PUT 403、显式撤销保持撤销。桌面及 390px 确认框、键盘复选和真实文件页检查通过。未知远端写入故障组合由 Go 回归覆盖，不将替身结果写成真实 Agent Space 的崩溃验收。

本轮连接修复最初部署为 `workspace-repair-20261012-2d1e350`；后续目录保护调整的当前构建见下文。迁移 6→6、应用 0 项。在线备份并保留旧二进制、SPA、冷态库和 spool；4 个用户、1 份草稿、12 个已有发布及全部旧用量事实逐行校验保留，运行中的新增用量允许追加。三个现有空间 ID 不变；管理检查、实际 104 字节旧验证文件读取通过，提供的 LAN 地址由本机浏览器实际访问，服务端 SPA 与构建字节一致。独立 Agent Space 的容器 ID、镜像和启动时间不变，未修改容量或重启。

证据根 `.data/workspace-review-20261011/connection-repair-20261012-1028/` 保存 `isolated-runtime-result.json`、`isolated-data-preserved.json`、`local-runtime-result.json`、`local-data-preserved.json`、Console 报告、浏览器图及更新备份。该目录含私有测试身份、凭据和数据库，不能整体公开。未重跑大文件、Android/手机、物理第二台 LAN 设备或正式发行验证。

## Files 保护目录与标准 DAV 边界修订（2026-10-12）

按更新后的 S1 合同及 Control Protocol §10.7.2，Files 只保护根下精确名称 `.agent-space`，标准 DAV 保持上游目录项和访问语义；机制见[实现参考](remote-workspace-implementation.md)。观察到混合列表拒绝、相似/嵌套名称误拒绝及 Relay 207 截断的 Red，再修复到 Green。全量 Go tests/vet、Console 类型检查、428 项生成材料的重复生成一致性及 core 合同材料校验通过。Admin/Client OpenAPI 只同步说明及文件摘要，字段、版本与数据库 schema 不变；生成改动仍未提交，不宣称相对 HEAD 的 drift 通过。

此次目录保护部署使用 `files-policy-20261012-2d1e350`，监听所有接口，Hub/Relay 版本及部署二进制摘要均核对一致；迁移 6→6、应用 0 项。真实管理员与 Client Files 根列表 200 且隐藏保护项，普通子目录及已有 README 读取 200；平台 DAV 根列表完整 207 并保留 `.agent-space`，该目录元信息与直连均 207，跨空间 403。验证所用临时设备已撤销。三个原空间 ID/绑定修订及全部前端资源字节保持不变，独立 Agent Space 容器、镜像与启动时间不变。

在线备份及冷态库/spool/旧程序保留于 `.data/workspace-review-20261011/files-policy-20261012/`；`runtime-verification.json`、`deployment-identity.json`、`data-preserved.json` 记录实际结果。4 个用户、1 份草稿、12 个发布、23 个 SecretVersion 及全部旧用量行逐行保全，数据库与外键检查通过，正常请求只追加新用量。此次未新增前端交互或重跑浏览器、Android、大文件及正式发行验收。

### 提交前完整审查（2026-10-12）

审查覆盖 Core 与架构仓库全部未提交变更。新增 Red/Green 确认并修复五处问题：应用请求缺少确认修订仍被接受、服务缓存不可用时冻结操作无法核实、Activation 争用导致更换永久失败、并发用户删除阻塞更换，以及并发相同幂等请求误报修订冲突。接口、fixtures、生成材料、测试脚本和文档同步；SQL schema 和产品/协议版本不变，既有前端布局调整保留。

全量 Go tests/vet、327 项 Console 测试、类型检查/E2E 类型检查/生产构建、81 项 tooling、重复生成一致及 core 材料校验通过。审查与本机后续交付证据位于 `.data/workspace-review-20261011/review-20261012/`；该目录含运行备份，不整体分发。未新增 Android、Agent Space 崩溃或正式发行验收声明。

## Spark 同版本更新（2026-10-14）

生产从 Core `0bd2cf8` 更新为固定 `431368a9b6b8bd739da9cee53165e081792a3fb0`，Architecture `6d251c218d0b3ca0672628735ebca2b03a2ab6ce`、Portal `425af0b6f8cd5056bff53db89c74a34cd51fdbc8`。产品版本仍为 `0.2.0-preview.23`；归档 SHA-256 为 `1393357b9518971053222fccd3bac0e29edaf8f04e6f596996a08382cb996fdf`，build hash 为 `sha256:e9f1ac77212726851a7628c48c45a59f559904c72ef0d915a9b1dab3203527bc`。包保持 Core candidate 身份，没有新增固定 Android APK 兼容声明。

更新包含近期工作区恢复、Files 保护、独立 WebDAV、凭据查看和用户云桌面链接。打包发现 Portal 尚未同步 Client 合同，修复其 WebDAV 用量识别、筛选及仅统计标记；观察两项回归测试 Red 后修复到 Green，不新增额度能力或价格结算。Core 本地启动修复确保普通重启保留管理员配置，仅新库/显式 reset 初始化 preset。

当前固定源码的全量 Go tests/vet、smoke、格式及生成漂移检查，Console 类型/E2E 类型检查与 344 项测试，94 项 tooling、103 项 Portal 测试、生产构建均通过。真实隔离 Hub/Relay 的 Admin 浏览器流程和 Portal 同源生命周期及宽窄屏用量检查通过；证据见 `.artifacts/production-20261014-*.log`。

ARM64 原包校验通过；生产备份副本及正式库均从 schema 5→6，只应用 `000006`。按原列逐表核对 42 张表，除追加迁移历史外原行与原值保持一致，integrity/foreign-key 检查通过；迁移前备份另由旧程序恢复检查。在线及停服最终备份均验证并离机保存，原程序和原包保留。

仅切换 Hub/Relay，两者实际执行新包并 ready；9 个其他 PM2 进程 PID/状态不变，7 个发布、1 份草稿、8 条定价及 11 个 SecretVersion 保全，config/secrets 摘要不变。公网 Discovery/live/ready/Admin/Portal 为 200，内部入口为 404；实际返回的 Admin/Portal index 和 JS/CSS 与包内原字节一致，生产登录页渲染正常。未执行生产管理员登录、生产 DAV 大文件写入、新 Android/手机消费或外部供应商资格验证；历史失败边界继续保留。现场、备份及 cutover 证据见私有 `.artifacts/production-20261014/` 和既有私有部署记录，不能整体公开。

## 已观测用量与 Spark 查询修订（2026-10-14）

Core 固定源码 `cc805be8ccd4520030980fa70f15820823017400`，Architecture `6a00798c6b7edbb86dcb8b6c40c644955995ab66`，Portal `19a39b838270e551cdd059e714e4976a7554447d`。版本仍为 `0.2.0-preview.23`；归档 SHA-256 `8769ad2dc8ec074828462f2b4cd76075a0fbd5d3693a15fdd9300f373bfcabd3`，build hash `sha256:ee1e0c5a7a4da9a8069554bf88026631d7bb04a461e3e0afa65a58c7ed634c1e`。本轮只改读取聚合及 Admin 展示，机制和通用协议核对见[用量参考](usage-budget.md#通用协议核对与验证边界)；持久化、Ent、迁移、spool、准入和结算代码不变。

已观察五能力混合计量及 HTTP 200 后异常的 Red，再修复到 Green；受影响 Go/HTTP/协议/合同测试与 vet、Console 345 项测试及类型/生产构建、Portal 103 项回归及构建、431 个生成文件重复生成一致均通过。11 项真实 Hub/Relay/生产 SPA 浏览器流程通过，覆盖末尾 usage 透明转发、已知与未知聚合、桌面/390px、刷新和定价入口。固定材料摘要同步后，生成 drift、core 材料核验、17 项发布工具测试及合同/导出测试通过。

Spark 从 `431368a` 切换到新包，仅重启 Hub/Relay。新旧程序读取同一备份均通过 schema 6 检查；正式库不执行 migrate，schema/migration identity 不变。停服状态全部 42 表原数据一致；恢复运行后 8 个发布、1 份草稿、8 条定价、9 个 SecretVersion 和配置/密钥摘要保持不变，integrity=ok、外键错误为零，9 个其他 PM2 进程 PID/状态不变。在线及最终完整备份均校验、离机保存；新旧包和恢复链保留。

公网 Discovery/live/ready/Admin/Portal 为 200、内部路径为 404；Admin/Portal index 及 JS/CSS 与原包逐字节一致。生产已登录浏览器刷新原时间范围后实际显示已观测 6082.3 万 Token（PARTIAL）、79 条传输异常/取消；工作区 718 条、未定价模型 154 条缺适用价格，价格和原 UNKNOWN/PARTIAL 事实未改写。证据在私有 `.artifacts/usage-production-20261014/`、`usage-*.log` 和既有私有现场记录，不能整体公开。本轮不新增外部供应商、Android/手机或大文件验收声明。

## 保留的失败边界

- 隔离工作区大文件 PUT 曾在约 50 MiB/64 MiB 传输中返回 502/503 与 UNKNOWN，记录在 `.artifacts/workspace-files-final/`、`.artifacts/resources-review/`。新空间重跑通过不证明间歇传输根因已修复；不得自动重放未知写入，生产验收前继续核查。
- 旧隔离 Agent Space 容器首次文件访问还曾发生 VM 启动 SQLite 外键失败，见 `.artifacts/workspace-review-push-final/`、`.artifacts/workspace-review-approved/`。新卷/新容器通过不证明旧环境已修复，也不能据此认定与大文件中断同源。
- 历史 Starter 设备详情加载曾超时，独立测试环境复跑通过，未确认根因；原失败见 `.artifacts/starter-device-real/` 及当轮 core-android 留档。
- 历史供应商授权错误、静音录音失败、LEVEL_0 未知语义量分别判断；ready/ACTIVE 或 deterministic Adapter 不证明真实供应商恢复、实际录音成功或资格通过。

## 阶段与发布边界

Gateway daemon、Gateway Control、Snapshot v6、Catalog 与三 daemon 生产监督仍为 S0.3 工作；S0.4/最终 S0 Exit 与完整 S1 compute/storage 各需自身固定候选和 gate。CAP 工具不是 ERX 结果生产者，当前单端回归不替代 Portal/Android 实际消费验证。

复核入口统一在[测试文档](testing.md)；固定发行、总合同关联实施状态和 Android 待完成项在[发布文档](release.md)；Spark 操作在[部署手册](s02-preview-deployment.md)。本页不新增阶段完成声明。
