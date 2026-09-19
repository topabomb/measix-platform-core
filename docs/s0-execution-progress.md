# S0 Platform Core 当前实现状态

> 状态日期：2026-09-19。本文是唯一 living implementation/stage status。执行清单见 [A1–P3 计划](android-platform-contract-readiness-plan.md)。后文保留各次执行证据，以本节最新状态为准。

## 审查后修正（2026-09-19，当前工作树）

对本工作树变更的审查发现并已修复的问题：

- **冻结证据链**：`CAP-C7-001` 不再因"清单文件存在"即判 PASS，改为校验清单自身的固定提交身份、干净来源、四个 OpenAPI/构建/fixture 哈希与完整证据 pin；`make clean-replay` 不再缺 `--manifest`，注释与 [release](release.md) 一致。
- **清理与回放**：`cleanupEnvironment` 改为同步执行（原先 3 秒延迟定时器在失败路径被 `process.exit` 丢弃，残留进程与临时目录）；`replay-freeze` 的工作区移到 `.artifacts/replay/`（终稿校验要回读其日志），并补入 `contract`、`system`（adapter/client）两个阶段。
- **浏览器 harness**：Phase B 四类能力流量失败现在是门禁失败而非 WARNING；用量等待登录失败不再静默返回；Playwright 报告缺失或无法解析时不再把陈旧产物当作本次证据。删除与 `e2e-harness.mjs` Phase A–D 完全重复、产物不被冻结证据消费、仍走旧直连 Relay 拓扑且无任何调用方的 `scripts/candidate-orchestrator.mjs`。
- **运行态呈现**："尚未发布配置"不再被头部健康指示器当成 Relay 故障（`unconfigured` 独立一档）；bundle 哈希缺失不再被判定为"未收敛"（契约中该字段可选，缺失应显示为 unknown）；轮询补上并发与乱序保护；补齐缺失的 `status.NOT_READY` 文案。
- **测试质量**：`SystemPage`/`OverviewPage` 基线改为契约合法的 `dbHealth: OK` 与真 64 位十六进制哈希，并补齐"已收敛"分支覆盖；`HealthIndicator` 不再 mock 整个 composable；资格脚本的 cancel/客户端超时改为可证伪断言，adapter 身份不再由被测上游的 `server`/`via` 响应头决定。

## 最新真实供应商实验与 Android 交接

当前局域网公共入口为 `http://192.168.31.235:9100`。第 5 版已真实发布并经协议客户端同步、应用回执和同源 Relay 调用：DeepSeek Flash 文本 SSE/工具/看图、MiMo WAV/流式 PCM、Firecrawl MCP initialize/202 通知/list/scrape、Qwen 3.8 Flash 文本/工具/看图、百炼 HTTP ASR 转写均成功。默认模型/TTS/ASR/企业助手和两个企业助手、三个入口均在 Snapshot 中。五类资源都有按资源归属的真实转发记录；`LEVEL_0` 不提供可靠 token/费用。Android 当前源码已读取 `defaultAssistantId` 并映射到 EnterpriseDefaults，但设备级进入空间及会话仍需维护方验证。本轮仅只读核对 Android 仓库。

四组凭据和协议结论见[真实供应商接入记录](real-supplier-integration.md)。百炼新加坡 Token Plan 直连 LLM/TTS/ASR/生图成功；Qwen Flash 图片与工具成功，具名强制工具调用需 `enable_thinking=false`。新增 `DASHSCOPE_HTTP_ASR`（HTTP JSON 音频 Data URI、`output.text`）的架构、Admin/Client OpenAPI、Hub 校验、Admin 编辑、共享 Snapshot/runtime 样例、Relay 凭据与载荷测试已完成。第 4 版发布 Qwen 与 HTTP ASR；实际管理界面和 Usage 审查发现 Firecrawl MCP 路由只允许 POST，历史 GET 请求被 Core 403 阻断。第 5 版补齐 POST/GET/DELETE，公共入口 GET 到达上游返回 405、无会话标识的 DELETE 返回 400；模型/TTS/MCP/Qwen/ASR 再次实测通过。Admin 资源编辑现让路由允许路径随资源路径变更，修正重复 ASR 提示、误写 DeepSeek 的通用提供商提示与用量页生硬术语；Hub 草稿校验也拒绝资源路径不在 binding allowlist 内或 MCP 方法不全。自动化 Go/Console 测试均通过；最终 Hub 重启后系统页确认活跃 Generation 5、Relay 已应用 Revision 10、状态已收敛。第 5 版的一次 Android 模拟器录音转写仍收到百炼上游 400；同一路由上的 Android 格式等价有效语音返回 200，8 秒纯静音 WAV 复现 400。静音是待核查的录音原因，不能将这次设备 ASR 调用算作通过。

2026-09-19 只读复核 Android 工作树时，`PlatformWire.kt`、Mapper 和文件识别执行器已支持 `DASHSCOPE_HTTP_ASR`，且有平台路由自动化样例。维护方仍须消费新共享包并用真机对第 5 版完成同步/应用回报与五类资源调用；详见[Android 接入说明](android-platform-integration.md#验证职责与交接清单)。这不是正式 S0.1 Freeze 或 Android 设备验收；S0.3 Gateway 仍按路线图后续推进。

## 本轮七项交付：公共接入与设备应用

用户已明确本轮必须支持普通 HTTP/IP。七项上游交付及逐项证据见 [公共接入交付计划](platform-access-delivery-plan.md)。已修订 Control Protocol 的统一公共 origin、HTTP Cookie 与显式设备应用报告语义。`public-origin`/`HUB_PUBLIC_ORIGIN` 替换旧 Portal 专用配置；Enrollment 响应携带该 origin，管理端 QR/复制不再取浏览器地址。Go 地址校验及 HTTP Admin Cookie 回归先复现失败再通过；前端 HTTP/IP 与接入材料 22 项测试通过。

新增 `PUT /api/client/v1/managed/applied` 当前协议、Session 报告字段及唯一初始化 SQL。有效发布/hash 验证、幂等重报、下载/304 不记录应用、报告不续 Session、撤销拒绝已有回归；设备列表显示应用状态与时间，发布页连接到设备列表。新的初始化结构使用重建数据库，不转换旧库。

2026-09-18 本轮真实浏览器验收已在 `http://192.168.31.235:9100/admin/` 的全新数据库完成：通过页面创建成员、生成二维码/复制完整资料、创建并测试/应用上游、编辑 Provider/Model/TTS/ASR/MCP/Assistant/Seed/Starter 和五项策略及默认资源、验证/预览/发布两次。第 2 次发布启用模型 TOOL 能力；当前五项个人资源准入全部开启。页面实测暴露并修复了普通 HTTP 缺少 Clipboard API 和 `crypto.randomUUID` 导致的复制/资源创建/上游应用失败；Core 使用 Quasar 现有工具，Portal Bridge 使用 HTTP 可用的 `getRandomValues` 生成请求 ID。修复均有实际 Red→Green。发布页误把已完成操作标成待确认、首次发布显示“第无版”及入口技术 ID 常显也已修正；重建后的浏览器已复验公共地址、已完成标签和设备应用报告；首次发布与入口展示另有组件回归。

同一环境协议客户端（非 Android 真机）使用页面生成的接入资料成功 Enrollment。管理界面观察到：快照下载后仍未知 → 第 1 次发布已报告应用 → 发布第 2 次后待更新 → 第 2 次已报告应用。旧 generation Runtime 返回 428；最新 generation 的模型 SSE、TTS、multipart ASR、MCP initialize/list/call 均 200，回退应用报告返回 409。上游为合成协议服务，不证明供应商生成质量；隔离结果在 `.data/access-preview/runtime-evidence.json`，凭据文件不提交。9100 为本轮环境；9000 的旧预览进程已停止，避免旧后端与新前端混用。

最新验证（2026-09-18）：完整 `go test ./...`、`go vet ./...` 通过；Admin 21 文件 128 项测试、typecheck 和生产构建通过；Portal 8 文件 103 项测试、remote/local 构建、交付测试及真实 Hub/本地包 6 项浏览器回归通过。`TestPublicHTTPAndHTTPSControlLifecycle` 用真实 HTTP/受信测试证书的 HTTPS 连接验证登录、接入、Snapshot、应用报告和 Portal 换票/Cookie/CSRF；`TestRealtimeASRWebSocketAdmissionAndFrames` 验证 ws/wss，未关闭证书验证。报告跨 Session、过期、回退、错误 hash、不续期和下载不算应用均有回归；Hub 实际重启后的管理页面仍显示第 2 次发布的已应用报告。HTTPS 的证据是自动化协议连接测试，不冒充 HTTPS 浏览器人工操作或 Android 真机验收。

2026-09-18 的交付包为 [android-integration-TzTeNh](../../measix-enterprise-portal/.artifacts/android-integration-TzTeNh/manifest.json)，当时 150 个文件已逐一校验摘要及清单，内含 Core 独立资料包 72 文件也通过独立校验。当时的 Portal sourceHash 为 `577379e9d5e78f05eda1916fc074418cdc1119d1cb407bc03544b3630d1d513a`，Core 资料 sourceHash 为 `1ec5190a4ab8df11375af82832a9e0700a0dcacd18584a4aa0654988cb1a2590`。交接说明已消除 wss-only 的旧描述；HTTP 使用 ws，HTTPS 使用 wss，端口保留。这些是当时工作树/交付内容身份，不是正式冻结提交；最新交接包应以本轮重新生成的 manifest 为准。

Android 工作树已有其他维护方新增的 PlatformControlClient/Mapper/reportApplied，本轮只读核对，未修改 Android。七项上游交付审计完成；正式阶段 Freeze、供应商生成质量和 Android 真机验收不由本轮测试替代。

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

Android 集成导出包含 72 个内容文件及 manifest，当前 sourceHash 以 `api/generated/android/integration/manifest.json` 为准。导出校验拒绝错误协议版本、漏文件、多余文件、篡改和路径逃逸。

## 本轮验证

| 检查 | 结果 |
| --- | --- |
| Core backend | `go test ./... -count=1` 通过；`go vet ./...` 通过 |
| Core candidate systems | `go test -tags=candidate ./test/system/scenarios/ -count=1 -timeout 15m` 通过；真实 Hub/Relay/SQLite + deterministic Adapter |
| Core Admin | 21 个 Vitest 文件、138 项测试通过；`vue-tsc --noEmit` 与 Quasar production build 通过 |
| Core browser | `node scripts/e2e-harness.mjs` 通过 Admin authoring/publish、四类 runtime traffic、usage/system 和 topology security；System 页面在干净 Chromium 中没有页面脚本异常 |
| Current schema | Atlas 对独立空库执行 apply/status：一份 SQL、37 statements、Pending 0；tooling 8 项测试通过 |
| Portal | 8 个 Vitest 文件、102 项测试和 2 项交付测试通过；format、remote/local production build 通过；6 项 Playwright（5 项 local + 1 项真实 Hub 生命周期）通过 |

Windows 当前 Go 环境未启用 CGO，`go test -race` 在测试启动前被 Go 拒绝。当前协议扩展后的全量 Go 测试、vet、Admin 测试与完整浏览器 harness 已运行；candidate system 最新运行通过（207.9 秒）。Portal 最新 102 项单元测试、2 项交付测试、两种生产构建及 6 项浏览器测试（5 项 bundled local + 1 项真实 Hub 生命周期）通过。没有执行 Android 编译/设备测试、付费模型/语音供应商 qualification、独立 clean-source rebuild/replay 或最终 Freeze。Firecrawl 官方免密钥 MCP 已完成真实调用，不能替代其他供应商证据。

## 管理员实际操作审查（本机独立预览）

2026-09-18 使用隔离的 `.data/admin-preview/hub.db`、生产 SPA、真实 Hub/Relay 和确定性 HTTP Adapter，通过管理台从空库完成：创建用户并生成完整的一次性接入资料；创建 Secret/Upstream、测试连接并 Apply；配置提供商、Model/TTS/ASR/MCP、默认资源、五项准入、企业助手/记忆种子/两个 Starter；保存、修正可导航的 Starter 校验错误、Validate、Preview、Review、Publish。Release Generation 1 Active、Relay revision 收敛，Hub/Relay `/ready` 均为 200。使用界面生成的资料完成 Client enrollment，取得 Snapshot v4 与独立 Enterprise Update Feed；客户端声明已应用 Generation 1 后 Managed State 允许执行。经 Relay 的 Model SSE、TTS、ASR 和 MCP initialize/list/call 均返回 200，Admin Usage 页面显示六条已转发请求；Adapter 没有可靠语义计量，因此成本保持 UNKNOWN。企业动态的 Markdown 预览、发布、撤回和再次发布走通，Feed ETag 变化而 Managed Generation 保持 1。

实际操作中修正：Starter 追加顺序、终态 Activation 的错误恢复提示、重复预览时随机变化的投影哈希、预览遗漏的 Assistant/Seed/Starter 说明、System 长哈希和上游状态条布局、策略开关计数与 Usage 起止时间标签。独立 Secret 创建后刷新页面无法复用、上游候选编辑时响应式对象克隆失败，也已补齐 Secret 元数据列表与创建/编辑选择器并修复。重复预览、发布后 Client/Feed 读取及 390px 窄屏 System/Resources 无横向溢出已在生产构建浏览器中复核。此预览使用本机 HTTP 与确定性 Adapter，只证明 Core/Console 的配置和客户端协议路径；它不是 Android 真实平台接入、供应商资格或 C6/C7 Freeze 证据。

随后从管理员理解成本重新走了一遍：在 Resources 中修改企业助手说明、保存修订 4、Validate、Preview 确认最终说明/指令/记忆/引用、Review 看到 1 项 Assistant 修改，再由界面发布到第 2 版并完成 Activation；概览和发布历史均显示第 2 版活跃。入口改成任务顺序与可展开诊断；资源默认收起完整路由关系，审查只列实际变更并可直接打开最终快照；上游基本连接与高级能力分层、协议枚举本地化；公告列表按安全渲染后的内容和本地时间呈现；发布历史以版本、状态、变更和本地时间为主。复核了 Users、Resources、Upstreams、Releases、Enterprise Updates、Usage、System 的 390px 页面宽度，均无横向溢出。以上只覆盖 Core 浏览器、Hub/Relay 和确定性 Adapter；Android 当前平台 source 的真实设备消费仍需其维护方完成。

## System 诊断面审查

管理员理解成本复查继续修正了资源编辑与预览：模型/语音/工具列表使用名称和有意义的摘要，系统 ID 与固定协议折叠；模型能力、认证归属及上游状态使用本地化标签，仍提交原协议枚举；策略预览显示允许/不允许及默认资源名称，未指定与失效引用分别提示；Starter 描述完整呈现。预览的协议边界说明归入技术详情，窄屏操作菜单增加“操作”标签并在选择后关闭，导航按钮名称改正。用户列表按账户名和本地化角色呈现，用户 ID 留在详情，Android 接入操作保留正常大小写。生产 SPA 已通过浏览器实际复核资源表单和策略预览；Resources 17 项、Users 10 项回归通过，完整 authoring/publish、四能力 traffic、usage/system、topology harness 通过。设备目前没有设备名称字段，仍需保留唯一设备标识供撤销时辨认，不能虚构可读名称。

无已发布配置、Relay 未就绪时，System/Overview 与头部健康指示器现在都明确显示待配置，不再把未应用的运行态判作已收敛，也不把"尚未配置"误报成 Relay 故障；未知 ingest lag/orphan 值不再显示成 0。Hub 本地健康检查和上游配置/应用状态已分开标注；健康探针失败也不会吞掉独立取得的系统状态。

System 当前合同分别暴露 currentActivation（APPLYING/UNKNOWN）和 lastActivation（COMPLETED/FAILED），在同一数据库快照内读取；UI 独立呈现，终态不会被在途操作遮盖。后端与页面测试覆盖两者同时存在及完成后的切换，生产浏览器确认没有在途操作、最近发布已完成。上游列表明确反映配置/应用状态，连通性由上游页显式探测。Gateway 状态与 Tool Integration/Catalog drift 属于 S0.3，不作为当前能力占位展示。

系统计量新增 semanticUnknownRequestCount，复用 Usage 的请求完整度规则，对全部保留请求计数，缺计量或任一 UNKNOWN 计入一次；独立于未匹配请求的记录数。空库、EXACT/PARTIAL、混合记录、多记录去重和无关联记录均有测试。真实浏览器确认本机 6 条请求计量未知，未匹配记录为 0；完整 harness 也断言新统计。lastRelaySeenAt 原先误标“最近协调”，现明确标为“最近成功读取 Relay 状态”，没有冒充协调执行时间。清理了 runtimecontrol 的三个无用途导入及占位引用。两份误生成文件已由用户删除；当前复查 backend/backend 下没有源码文件。

Relay 构建标识已从运行进程通过 private status 传至 Hub/Admin；开发构建显示 dev，读取失败显示未知，不借用 Hub 版本或保留旧观察。当前 private status 必填 buildVersion，Hub 拒绝缺失/空值，不提供旧状态格式兼容。Relay App/Control Handler 合并为显式传入构建标识与计量依赖的单一初始化入口。测试覆盖 apply 前版本读取、认证边界、Hub 与 Relay 版本不同、Relay 失败后不保留缓存；完整浏览器 harness 和本机 System 页面均确认 Relay dev 与就绪状态。

发布与连接检查的真实性复查：移除前端按 APPLYING/errorCode 猜测 validating/staging/applying/finalizing 的进度阶段，发布页只呈现服务端当前 Activation state 与可恢复操作；当前 wire 没有细分阶段，不能把 UI 推测作为已完成证据。Upstream Test Connection 现在只报告无凭据 HEAD 的连通性、延迟及实际 HTTP 状态，删除把 transportCapabilities 原样复制为 verifiedCapabilities 的虚假验证字段。200/401/405/503 均有真实 HTTP 回归；本机浏览器确认适配器基础路径返回 404 时显示“可达 + HTTP 404”及检查边界，不宣称模型或流式能力验证成功。

## Android 边界与下一步

本次范围由用户明确为 S0.1/S0.2 全部能力及协议扩展，Gateway 按 S0.3 后续推进。当前 Core 可表达、发布并转发四种模型、四种 TTS（SYSTEM_TTS 不经 Relay）、三种 ASR 和 Direct MCP；实际管理员发布与公共调用证据见 [验证记录](admin-provider-verification-plan.md)。新增用量 `requestCompleteness` 由 Hub 在同一筛选和读事务中统计完整请求，替代前端错误统计 meter 汇总项；真实预览库已验证 25 条缺语义计量请求显示 25 未知。

最终人工复查已修复用量页日期输入、资源名称与信息层次：日期用本地选择器并转换为 UTC 查询，名称从请求所属不可变 Release 批量读取；技术编号筛选折叠，单条详情可筛选同一资源。浏览器实际确认 Firecrawl 名称与 4 条筛选结果；移除借用汇总成本的伪单请求成本。用户详情设备编号已缩短并折叠完整标识；四个页面的刷新按钮已补可访问名称。System 的 current/last 操作已按当前合同实现与验证。用量查询增加请求序号隔离：筛选变化清空旧数据，过时的成功、错误和分页响应均不得覆盖当前结果；三个响应乱序测试通过。

Android 仓库仍由其维护方独立修改，本轮保持只读。其当前在途工作不能由 Core/Portal 的 Green 代替。Android 维护方下一步应消费上述 exact 导出，保持 Snapshot v4/五项策略/Bridge v3/local-read v2 解析一致，并完成真实 Platform source、远端 Relay 四 profile、428/刷新/退出和物理设备互操作证据。

S0.3 的 Enterprise Tool Gateway、Snapshot v5、真实生产 supervisor/package 和后续 User Sync 不在本批实现范围，也没有预建空模块或兼容入口。

## 独立源码回放工具

已将重复的 runtime-only 回放替换为固定 Core/architecture 提交的独立检出、锁定依赖安装、契约再生成、生产构建摘要核对和完整测试回放。失败停止并保留日志，不覆盖候选或既有证据；正式清单单独生成，校验候选字节摘要、重建 facts、全部必需步骤及其日志摘要。真实临时 Git 仓库测试验证工作树修改/私有文件不会进入检出，失败命令不会执行后续步骤。工具功能测试通过不代表本工作树已完成正式 C7；当前尚无满足全部固定来源及真实四能力 qualification 条件的新候选，完整 clean-source gate 未执行。具体命令见 [release](release.md)。

测试上游身份摘要现覆盖完整 Go adapter/client 目录和浏览器测试上游源码，修复新增协议文件变化不影响旧三文件摘要的问题。工具测试共 16 项通过；检出测试额外验证源仓库 HEAD 已前进时仍取指定旧提交。删除约 600 行重复回放流程，复用完整浏览器 harness；本轮没有改变客户端 wire，也没有修改 Android。

## S0.2 责任与交付复核

本轮范围已明确为 S0.1/S0.2 全部能力及协议扩展；Enterprise Tool Gateway 保持 S0.3 后续工作，不作为当前实现范围。阶段通过仍按对应门禁声明。

以下是对现行 ERX 规格的责任核对，不是整期通过声明。没有用服务端测试替代原生行为。

| 要求组 | Core / Portal 当前证据 | 仍须由 Android 完成的部分 |
| --- | --- | --- |
| ERX-C0、POL-001、CONTRACT-016 | 共享原文 enrollment/Snapshot/reference fixtures；compiler hash/字段/引用闭包；完整导出独立校验；Client HTTP pending、ETag、撤销与退出测试 | 平台 source 消费同一输入摘要并验证原子应用、拒绝错误候选 |
| ERX-C-001–009 | Admin 实际发布助手/记忆种子/入口；staged release 与 preview 保留内容、作者顺序和稳定排序 | 只读 seed 与可写本域记忆隔离、点击预填不自动发送、个人域不可见 |
| ERX-UPD、ERX-B | 共享日期向量、DST/ETag、并发 revision；Admin 发布/撤回；真实 Hub Portal 列表/详情/失效及独立 Feed generation | 原生 Feed 消费与本域 read marker；原生入口联动 |
| ERX-REALM、ERX-TRG | Enrollment/Refresh 并发轮换及重建恢复、七天 idle、Portal 不续母 Session；Relay generation/428 零转发 | Realm 数据隔离、企业进入/前台/交互前检查、平台 source、冻结执行上下文 |
| ERX-PORTAL-001–016、PHONE | Bridge accessor 重建、取消/迟到回复、媒体分块、状态恢复、日期/缓存；本地及真实 Hub 浏览器回归；Portal Cookie/Origin/CSRF 边界 | WebView 实际来源/frame 授权、原文档 ReplyProxy、站点清理和硬件权限/采集生命周期 |
| ERX-INIT、DATA、SIM、POL-002–005、JOIN | 当前唯一 schema、五项策略和完整接入材料已交付；无数据库转换或旧策略分支 | 原生初始化、用户数据与本地资源准入、模拟来源与平台来源隔离、相机扫码/粘贴统一解析 |

清理了 capability 测试里将服务器投影命名为“只读记忆”“点击预填”“个人域隔离”的错误证据标签；删除与 canonical order 测试重复的 seed 测试，Starter 测试实际核对 title/prompt/description/sortOrder/引用完整投影。该包回归通过，不冒充 ERX-C-009 的 Android 域隔离证据。

交付包另外补入根目录下 Core 接入说明及所引用的 client-integration/problem 样例，修正 Portal 交接链接在解包后找不到目标的问题。独立检查确认该说明的 13 个本地链接均存在，文件清单无遗漏或额外内容。

当前可交付包、摘要和 Android 工作区状态统一见本文顶部，不再维护上一轮包路径及过时实现结论。

## 同源接入闭环复核

发现此前本机脚本从 Hub 9000 接入后直连 Relay 9002，与 Discovery 返回同源 `/runtime/v1` 的实际消费路径不一致。现补入 [Caddy 配置与启动说明](operations.md#one-public-origin)，预览入口仍为 9000，Hub 移到 9004，Relay 保持 9002；9001/9003 仅内部通信。公开入口同时提供 Admin、Portal、Discovery、Client 与 Runtime，并拒绝 `/internal` 及子路径。

浏览器 harness 改为从公开入口读取 Discovery、执行 enrollment/state/Snapshot，并使用 Snapshot 的 runtimePath/model key 调用四能力；五阶段完整回归通过。独立代理测试先复现 Discovery 404，再验证路径、query、POST 和认证头正确转发；worker 改为等待两个真实监听就绪再报告 ready。工具测试共 14 项通过。

真实 Caddy 2.11.4 入口上完成四模型流、三云端 TTS、HTTP ASR、两种实时 ASR、Firecrawl 官方免密钥 MCP，以及 Portal grant 303 换票/会话 200、静态页面 200、私有路径 404。实时 ASR 暴露测试 peer 的 DashScope session.finish 后过早关闭 TCP；补“等待客户端 Close 确认”的失败回归，修复后 adapter 全包及实际双代理连接完成验证。没有为此改动 Relay 协议或 Android。供应商内容除明确标识的 Firecrawl 外仍为合成结果；SYSTEM_TTS 仅下发验证，设备播音、公网 TLS 与正式 Freeze 未由这些证据替代。

## 本次目标交付审计

| 用户要求 | 当前可复核交付 | 判定边界 |
| --- | --- | --- |
| 管理员能理解并完成配置与发布 | 八个页面实际审查；名称/分区/详情编辑；保存、验证、Preview、发布及用量浏览器闭环 | 本地管理端已验证；不是仅 API 测试 |
| DeepSeek / CLIProxyAPI 与另外两种模型协议 | 四协议字段、编译样例、公开 Runtime 流、工具往返与认证测试 | 协议测试通过；未使用付费账户或实际 CLIProxyAPI 部署 |
| MiMo 及 Android 四种 TTS | 标准/音色设计共享样例、三云端请求与音频响应、SYSTEM_TTS 企业默认参数 | 系统引擎播放和生成音质由设备/厂商验证 |
| 三种 ASR | HTTP multipart、OpenAI/DashScope WebSocket 参数、事件、取消和关闭握手 | 合成音频与转写；不声明真实识别准确率 |
| Firecrawl Direct MCP | 真实免密钥服务 initialize、通知、工具列表与公开页面抓取；助手引用 | 不包含 Gateway 或付费账户资格 |
| 助手、记忆种子、入口、策略、动态、Portal | 当前 Snapshot/Feed/Bridge、编译/发布测试、Portal 单元与真实 Hub 浏览器回归 | 本域记忆、聊天隔离和硬件属于 Android |
| 可独立交给 Android 的协议与静态包 | 接入说明的六步实施清单；150 文件 Portal 包、内含 72 文件 Core 资料；再次逐文件比对包及现存源码无差异 | sourceHash 是工作树身份；不是冻结提交 |
| 当前唯一版本、清除无意义旁路 | 单一初始化 SQL、严格当前 Snapshot、删除旧 diff/重复流程/误导测试命名 | 两份误生成的 `backend/backend/internal/wire/{adminapi,clientapi}/*.gen.go` 已由用户删除，重新检查均不存在；正常生成源码保留 |

下一步交接应直接使用 [Android 实施清单](android-platform-integration.md#验证职责与交接清单)，而不是让 Android 猜测资源协议或上游密钥。Android 维护方正在独立接入 Platform source，应使用当前工作区实现核对交接，不能把本轮上游交付说成 Android 已接通。正式阶段 Freeze 与设备验收继续按原门禁执行。

本次本地交付收尾：用户删除误生成文件后，确认两个错误路径均不存在、两个正常 wire 文件仍存在；重新执行 `go test ./... -count=1`、`go vet ./...`、14 项 Node 工具测试、72 文件 Core 独立导出校验和 `git diff --check` 全部通过。Android 工作树保持干净。架构/Core/Portal 本轮实现、管理员审查、协议扩展与交接资料已交付；没有提交、发布或宣布正式 S0.1/S0.2 Freeze。付费供应商测试按用户确认的官方协议加严格合成上游方式完成，真实 Firecrawl 证据单列；Android 设备与公网部署验收继续由后续接线承担。

## Android 接入可用性复核

重新核对路线图及 Foundation/Realm/Android authority 后，补充明确“正式阶段 Exit 顺序不禁止当前合同提前联调”：v4 的四模型、四 TTS、三 ASR、Direct MCP 可以现在适配，不等待 Gateway，不制造空 Gateway readiness 或 v4/v5 双轨。S0.4 的正式冻结条件保持不变。当前定位是 S0.1/S0.2 上游候选已具备，Android 真实平台 source/设备闭环仍待维护方完成；不是全部 S0.2/S0.4 已通过。

接入说明新增个人空间体验边界与逐项阻碍处理：手机可信 HTTPS、真实上游配置、五项 allowLocal 策略、辅助模型选择、认证/428 恢复、10 MiB 请求/连接限制。现场读取 generation 9 确认四模型/四 TTS/三 ASR、一个助手、两个入口、两个 MCP，五项 allowLocal 均为 false；这是隔离测试配置，不是可忽略的实际产品默认。新增四协议图片输入与历史消息逐字节转发测试通过，连同供应商、工具和 WebSocket 相关回归通过；它证明 Relay 不剥离图片，不证明模型视觉能力或设备编码。

资料同步后的最新交付目录为 [android-integration-RxyrgW](../../measix-enterprise-portal/.artifacts/android-integration-RxyrgW/manifest.json)，150 文件摘要验证通过；Core 72 文件 sourceHash 为 `950a41fa33b94148095894a1f1962faeb21e5e2035c31aa37baa5e1c87d90371`。Portal 页面/Bridge 无变化，沿用同一已验证生产构建；本轮变化是架构说明、接入指南及新增 Relay 回归。导出独立验证与篡改测试通过，Android 未修改。
