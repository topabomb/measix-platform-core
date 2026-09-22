# S0 Platform Core 当前实现状态

> 状态日期：2026-09-22。本文是唯一 living implementation/stage status。后文历史证据以本节最新状态为准。

## 枢策品牌、管理员改密与模型标识映射（2026-09-22）

Admin 产品壳已按官网统一为“枢策 Orchelm · 企业智能体治理与协同平台”和紫色品牌体系，保留原有紧凑全宽工作台及宽窄屏同一状态。登录管理员可在桌面或移动账号菜单验证当前密码并修改自己的密码；成功后服务端原子撤销该账号全部 Admin Web Session，页面清理本地会话并要求使用新密码重新登录。明确错误使用稳定 Problem code，并由中英文 locale 给出面向用户的提示。

Model 草稿新增可选 `publishedModelKey`。留空按 `upstreamModelKey` 下发；配置别名时，Client Snapshot 仍使用既有字段承载有效下发标识，Android 契约形状和版本不变，也不会获知真实上游 key。Hub Runtime Control 为四种模型协议生成显式双向映射；Relay 对 OpenAI Chat/Responses/Anthropic 只替换顶层 JSON `model`，对 Google 只替换精确模型路径，并在转发前拒绝错误别名、重复字段、非 JSON/压缩请求或不匹配路径。数据库无需新增表或列：草稿与不可变 Release 内容原本就是 JSON 聚合，旧内容缺字段时由编译器执行确定性回退。

## S0.2 生产用量与用户额度闭环（2026-09-20，当前状态）

Core 当前实现 14 个受管协议 profile（含 OpenAI 与 DashScope 两种同步 text-to-image）的有界生产观察与语义计量、durable spool/幂等修正/待核对恢复、日周月累计预算准入和真实结算。HTTP 压缩响应只在私有有界观察副本中解码，代理原始字节与响应头不变。Admin 已提供 MODEL/TTS/ASR/MCP/IMAGE_GENERATION 五类能力的用户额度编辑、live-linked Budget Templates、当前状态、趋势/分布/请求明细与 reconciliation；图片只有 REQUESTS/REQUESTED_IMAGES，不存在资源级预算。Portal/Client 本人接口只按认证主体查询有效能力预算，不暴露模板身份。后文早期“真实请求均 UNKNOWN”“只有合成上游”的记录仅是当时快照，不代表当前实现。

受管 Image Generation 使用独立 `img_*` 身份、`OPENAI_IMAGES_GENERATIONS` / `DASHSCOPE_MULTIMODAL_GENERATION` typed profile 和固定 Runtime binding。Relay 按已发布协议只验证固定同步 JSON 形态并观察 `n`/`size`，不解释 prompt/model、不转换正文、不存储图片。Snapshot v4 和当前初始化 SQL 直接纳入这些字段与表；Core 未发布，因此没有协议版本递增、数据库迁移链、兼容层或回填逻辑。旧 Android 本地持久化数据的加法升级由 Android 自身容忍缺字段；Core wire 仍按当前合同严格。

Admin 的用户删除采用精确用户名和原因确认、deny-first 状态机及完整私有数据清理。旧 access/runtime 和 refresh credential 通过不可逆摘要 tombstone 统一返回 `enterprise_identity_deleted`；审计不保留可恢复身份。真实浏览器流程已覆盖额度配置、用量分析和删除交互；真实 Core 分发链路已覆盖 DeepSeek、Qwen、MiMo、DashScope ASR、DashScope 文生图与 Firecrawl MCP。Android 模拟器已用一次性接入资料完成企业域选择、Snapshot 同步、`wan2.7-image` 默认解析、Core Relay 原生 DashScope 请求、安全 URL 下载和 `GeneratedMediaStore` 持久化；Core 对该请求完成 200 结算并记录 REQUESTS=1、REQUESTED_IMAGES=1。普通 connected 门禁中的付费供应商用例仍按设计跳过，不能据此扩大为全部供应商设备验收。

最终跨端复核另发现并修复两处闭环缺口：Portal 已展示图片协议筛选，但 Core Usage filter 尚未接受 `IMAGE_GENERATION` 与两种图片协议；DashScope Snapshot 已生成导出，但未进入 canonical hash 复算与 Android 交接清单。修复后真实 Portal + Hub Chromium 用例实际选择资源和 DashScope 协议并取得成功查询，新 Snapshot 的 source/export 字节一致并由 Core 复算 `snapshotHash`。

完整用户生命周期按 principal ID 而非 username 判定。删除 COMPLETED 后，管理员可复用 username 创建全新的 `usr_*`；新用户不继承旧 Device/Session、额度或用量，同一 installation 因旧 Device 已删除而可凭新一次性码重新接入。旧 principal 与 credential tombstone 永久保留拒绝能力。浏览器 harness 已覆盖删除、同名重建、默认额度未继承、新 Enrollment 与再次删除；Core 组件测试额外覆盖同 installation 兑换、旧 Access/Refresh 持续拒绝和新 credential 正常认证。

## 审查后修正（2026-09-19，当前工作树）

对本工作树变更的审查发现并已修复的问题：

- **冻结证据链**：`CAP-C7-001` 不再因"清单文件存在"即判 PASS，改为校验清单自身的固定提交身份、干净来源、四个 OpenAPI/构建/fixture 哈希与完整证据 pin；`make clean-replay` 不再缺 `--manifest`，注释与 [release](release.md) 一致。
- **清理与回放**：`cleanupEnvironment` 改为同步执行（原先 3 秒延迟定时器在失败路径被 `process.exit` 丢弃，残留进程与临时目录）；`replay-freeze` 的工作区移到 `.artifacts/replay/`（终稿校验要回读其日志），并补入 `contract`、`system`（adapter/client）两个阶段。
- **浏览器 harness**：Phase B 五类能力流量失败现在是门禁失败而非 WARNING；用量等待登录失败不再静默返回；Playwright 报告缺失或无法解析时不再把陈旧产物当作本次证据。删除与 `e2e-harness.mjs` Phase A–D 完全重复、产物不被冻结证据消费、仍走旧直连 Relay 拓扑且无任何调用方的 `scripts/candidate-orchestrator.mjs`。
- **运行态呈现**："尚未发布配置"不再被头部健康指示器当成 Relay 故障（`unconfigured` 独立一档）；bundle 哈希缺失不再被判定为"未收敛"（契约中该字段可选，缺失应显示为 unknown）；轮询补上并发与乱序保护；补齐缺失的 `status.NOT_READY` 文案。
- **测试质量**：`SystemPage`/`OverviewPage` 基线改为契约合法的 `dbHealth: OK` 与真 64 位十六进制哈希，并补齐"已收敛"分支覆盖；`HealthIndicator` 不再 mock 整个 composable；资格脚本的 cancel/客户端超时改为可证伪断言，adapter 身份不再由被测上游的 `server`/`via` 响应头决定。
- **死代码与死文案**：删除只被自身测试引用、任何页面都未使用的 `console/src/stores/operationalApply.ts` 及其专属测试块；删除 64 个中英文均未引用的 i18n 键（en/zh 各 773 → 709，删除后仍逐键对称）。判定用真实语言模块的键集而非文本推断，因为该文件缩进不统一（`experience` 块为 1/2/4 空格混用），按缩进解析会得出错误结论。
- **修正"校验层做减法"的未收尾处**：上一轮移除 `atlas.sum` 时，`cmd/devmigrate` 里的 `migrate.Validate(dir)` 被留在原地，而该调用正是要求 `atlas.sum` 存在，导致 `start-real-device-preset.ps1` 的数据库初始化必然失败（`invalid migration directory checksum: checksum file not found`）。上轮"删除该文件后已跑过测试"不构成证据：`cmd/devmigrate` 的三个测试都直接调用 `initializeSchema`，从未经过 `run()`，结构上不可能覆盖该校验。现移除该悬空调用（开发库与 SQL 的漂移仍由 `devmigrate_revisions` 中的文件名与 sha256 检出），并补上直接调用 `run()` 指向真实 `backend/migrations` 的测试；该测试在还原修复后确实复现同一错误。同时更正 `.gitignore` 中"atlas.sum 必须提交"的过时注释。
- **修复冻结证据链的两处断点**：`freeze-manifest` 要求 8 个产物及其 meta 全部匹配被冻结提交，但其中两个没有可用生产者。`candidate-test.json` 的唯一生产者是 `3501063` 在一次 Makefile 重整中删掉的 `freeze-gate` 目标，而约 30 条 CAP 场景仍以它为证据，等于冻结路径不可达；现恢复为 `make collect-candidate`，与既有 `collect-artifacts` 同一模式。`scripts/collect-baseline.mjs` 只写产物、从不写 meta，即使真实采集也无法满足冻结；现与 `checks.mjs`/`collect-adapter-qualification.mjs` 一致地写入 meta。`docs/release.md` 补上冻结前必须执行的 8 项证据采集命令（此前完全没有记录）。
- **新增两个契约测试防复发**：`cli-contract.test.mjs` 校验仓库工具（`package.json` 脚本、Makefile、`*.ps1`）传给每个 `./cmd` 的参数确实被该命令注册——`cmd/*` 的 CLI 接线此前零覆盖，参数改名只会等到有人跑脚本时才暴露；`freeze-producers.test.mjs` 校验每个必需冻结产物都有生产者，且该生产者同时写入 meta。两者都在还原对应修复后复现失败。另修正 `Makefile` 缺失的 `.PHONY` 声明（`console-build`、`tooling-test`）、`real-device-preset` 中把文件路径错命名为密码的 `MEASIX_REAL_DEVICE_ADMIN_PASSWORD`（改为 `_FILE`），并清理 `cmd/migration-checksum` 遗留空目录。
- **文档同步**：`testing.md` 与 `development.md` 中"`make ci` 会先重新生成再查漂移"、"`make generate` 覆盖 current-schema checksum"、"schema 变更同时更新 SQL 与校验和"等表述已随校验层移除而失真，现按实际门禁范围改写，并明确 Green `ci-gate` 不证明产物与源一致。
- **本地产物观察（未提交，仅供排查参考）**：`.artifacts` 现有 8 份 meta 中 5 份（backend/system/console/candidate-test、resource-baseline）的 `command` 为占位值 `"test"` 且时间戳集中在同一秒，并非任何采集器输出；冻结会以 `source mismatch` 拒绝，不会将其洗成证据。
- **过期开发库的处理补齐**：`npm run device:real` 在数据库初始化处报 `non-current schema/checksum`，这是设计内行为——当前初始化 SQL 在 `4e81020`/`bda5c28` 改过内容，而本地库停留在旧版本（已确认四个本地库记录同一文件名下的三种不同 sha256，工作区文件与 HEAD 字节一致）。但该路径此前只能靠人工判断：`scripts/dev-setup.mjs` 的注释写着"过期开发库会被删除并重建"，代码却只调用 `devmigrate`，而失败即退出——承诺没有实现。现按注释与 `docs/database-migrations.md` 的政策实现：仅当 `devmigrate` 报 `non-current` 三类消息之一时，删除 `.data/hub.db` 及其 `-wal`/`-shm` 后重建；"无法识别"的库不自动删除。判定文案抽到 `scripts/lib/obsolete-database.mjs`，并由 `dev-setup-obsolete.test.mjs` 断言其仍逐字出现在 `cmd/devmigrate/main.go` 中，避免 Go 侧改文案后该路径静默失效。真机预设的初始化失败提示同时补上 `npm run device:real:reset` 指引。
- **控制台列表分页与用量呈现复核**：逐页复核了全部列表的分页方式与筛选能力。用量请求的 keyset 分页在契约与后端都是真实存在且健壮的（游标绑定当前筛选集，`pageScope` 校验），但**界面上毫无分页痕迹**：页大小硬编码 200 而实际只有 91 条记录，`nextCursor` 恒为空，"加载更多"从不渲染，且加载按钮只在 Summary 页签、位于列表卡片之外。同时请求行**不显示用户与设备**（只在详情弹窗里有 `userId`），按用户筛选是一个藏在折叠区、要求手打 `usr_` 标识的文本框 —— 从可用性看"不能按用户筛选"成立。另有若干配置型列表做了服务端分页却没有搜索，属最差组合。
- **本轮改动**：`RequestUsageView` 增加 `userDisplayName`（required，因 `user_id` 是 NOT NULL 外键故必然可解析）与 `deviceName`；`/users` 增加 `query` 检索（重排 YAML 锚点归属，确认未污染 `/releases` 与 `/enterprise-updates`）；后端新增批量身份解析并与资源名统一收敛到 `enrich`，消除"列表与详情各写一份富化"的漂移（详情端点原本只补齐资源名）；抽取 `UsageRequestList.vue` 供用量页与用户详情共用，显示身份列、已加载计数、页大小与有界滚动容器；用量页增加用户选择器、默认 24 小时时间窗、文本筛选防抖（原先每次按键都触发一次全历史聚合）与 URL 同步；Users 页增加按名搜索、该用户用量摘要与明细入口，并修掉打开 A→B 时显示 A 设备的竞态（原实现不清空、无守卫）；激活历史在后端与契约层加上界；删除死代码 `buildCursorQuery`。
- **一处自我纠正**：我最初用行式扫描读契约，未跟进 YAML 锚点，据此得出"`/usage/requests` 没有任何查询参数"，与事实相反。锚点式契约必须解析引用后判断。
- **验证**：`userDisplayName`/`deviceName` 与用户搜索各补 Go 测试；前端新增一项测试文件覆盖"按该用户与显式时间窗取用量""A→B 切换不串设备""搜索参数与防抖"。三处修复分别还原后三项测试均以精确信息变红。go build/vet/test 全绿，`vue-tsc` 通过，前端 140/140，i18n 双语 728/728 对称，gofmt 与 `git diff --check` 干净。
- **浏览器实测复核**：在 harness 保留的真实环境（生产 SPA + Hub/Relay）上逐页截图审查，发现并修掉四处只在真实渲染下才暴露的问题。一是用量页移除旧 ID 筛选后遗留的误导说明文字「按用户、资源或上游编号筛选」，该处已不再按用户筛选；二是「每页条数」原在筛选行，已移入请求列表头部与「已加载 N 条」同处；三是用户选择器归位到查询筛选行，不再与日期区间混排；四是用户详情弹窗改为限高 90vh + 正文滚动，此前新增的用量区块把「关闭」挤出视口。弹窗首次修复用 `q-card class="column"` 未生效（头部与正文左右并排），改为显式 `display:flex; flex-direction:column` 并给滚动子项 `min-height:0`。改动后浏览器门禁重跑仍 Phase A–E 全绿。
- **交付包收敛（高内聚低耦合）**：Android 导出不再内嵌 29 份架构文档正文，只保留客户端实际消费的 Client OpenAPI、Portal Bridge 契约、共享 fixtures、428 样例与接入说明；`api/fixtures/problem` 由整目录改为显式文件（Admin 专用的 `stale-draft-revision` 不再交付）。架构文档按文档名与章节引用，权威仍在其自有仓库。导出因此不再依赖任何兄弟仓库。
- **校验层做减法**：移除 `atlas.sum` 与维护它的 `cmd/migration-checksum`、`make schema`/`schema-replay`、CI 的 Atlas 安装步骤；CI 不再执行 `make generate` 与 `generated-drift`（两者保留为本地命令）。导出包不再生成 `manifest.json`/`sourceHash`/`verify.mjs` 自校验层，包内容由生成脚本直接从源文件复制。`make ci` 现在只跑测试。这些比较曾需要工作区与生成环境逐字节一致（本地 41 个文件违反 `.gitattributes` 的 LF 规则即导致两次 CI 失败），维护成本高于其收益；相应的迁移验证改由真实 SQL 的 Go 测试承担。

## 最新真实供应商实验与 Android 交接

当前局域网公共入口的具体值仅保存在忽略文件。第 5 版已真实发布并经协议客户端同步、应用回执和同源 Relay 调用：DeepSeek Flash 文本 SSE/工具/看图、MiMo WAV/流式 PCM、Firecrawl MCP initialize/202 通知/list/scrape、Qwen 3.8 Flash 文本/工具/看图、百炼 HTTP ASR 转写均成功。当前合同现已包含十项独立可选默认值及两个企业助手、三个入口；已有第 5 版证据早于这五个辅助默认字段，不能作为本次十项 UI/设备验收。五类资源都有按资源归属的真实转发记录；`LEVEL_0` 不提供可靠 token/费用。

四组凭据和协议结论见[真实供应商接入记录](real-supplier-integration.md)。百炼新加坡 Token Plan 直连 LLM/TTS/ASR/生图成功；Qwen Flash 图片与工具成功，具名强制工具调用需 `enable_thinking=false`。新增 `DASHSCOPE_HTTP_ASR`（HTTP JSON 音频 Data URI、`output.text`）的架构、Admin/Client OpenAPI、Hub 校验、Admin 编辑、共享 Snapshot/runtime 样例、Relay 凭据与载荷测试已完成。第 4 版发布 Qwen 与 HTTP ASR；实际管理界面和 Usage 审查发现 Firecrawl MCP 路由只允许 POST，历史 GET 请求被 Core 403 阻断。第 5 版补齐 POST/GET/DELETE，公共入口 GET 到达上游返回 405、无会话标识的 DELETE 返回 400；模型/TTS/MCP/Qwen/ASR 再次实测通过。Admin 资源编辑现让路由允许路径随资源路径变更，修正重复 ASR 提示、误写 DeepSeek 的通用提供商提示与用量页生硬术语；Hub 草稿校验也拒绝资源路径不在 binding allowlist 内或 MCP 方法不全。自动化 Go/Console 测试均通过；最终 Hub 重启后系统页确认活跃 Generation 5、Relay 已应用 Revision 10、状态已收敛。第 5 版的一次 Android 模拟器录音转写仍收到百炼上游 400；同一路由上的 Android 格式等价有效语音返回 200，8 秒纯静音 WAV 复现 400。静音是待核查的录音原因，不能将这次设备 ASR 调用算作通过。

2026-09-19 只读复核 Android 工作树时，`PlatformWire.kt`、Mapper 和文件识别执行器已支持 `DASHSCOPE_HTTP_ASR`，且有平台路由自动化样例。维护方仍须消费新共享包并用真机对第 5 版完成同步/应用回报与五类资源调用；详见[Android 接入说明](android-platform-integration.md#验证职责与交接清单)。这不是正式 S0.1 Freeze 或 Android 设备验收；S0.3 Gateway 仍按路线图后续推进。

## 本轮七项交付：公共接入与设备应用

用户已明确本轮必须支持普通 HTTP/IP。七项上游交付及逐项证据见 [公共接入交付计划](platform-access-delivery-plan.md)。已修订 Control Protocol 的统一公共 origin、HTTP Cookie 与显式设备应用报告语义。`public-origin`/`HUB_PUBLIC_ORIGIN` 替换旧 Portal 专用配置；Enrollment 响应携带该 origin，管理端 QR/复制不再取浏览器地址。Go 地址校验及 HTTP Admin Cookie 回归先复现失败再通过；前端 HTTP/IP 与接入材料 22 项测试通过。

新增 `PUT /api/client/v1/managed/applied` 当前协议、Session 报告字段及唯一初始化 SQL。有效发布/hash 验证、幂等重报、下载/304 不记录应用、报告不续 Session、撤销拒绝已有回归；设备列表显示应用状态与时间，发布页连接到设备列表。新的初始化结构使用重建数据库，不转换旧库。

2026-09-18 本轮真实浏览器验收已在现场局域网公共入口（具体值仅保存在忽略文件）的全新数据库完成：通过页面创建成员、生成二维码/复制完整资料、创建并测试/应用上游、编辑 Provider/Model/TTS/ASR/MCP/Assistant/Seed/Starter 和五项策略及默认资源、验证/预览/发布两次。第 2 次发布启用模型 TOOL 能力；当前五项个人资源准入全部开启。页面实测暴露并修复了普通 HTTP 缺少 Clipboard API 和 `crypto.randomUUID` 导致的复制/资源创建/上游应用失败；Core 使用 Quasar 现有工具，Portal Bridge 使用 HTTP 可用的 `getRandomValues` 生成请求 ID。修复均有实际 Red→Green。发布页误把已完成操作标成待确认、首次发布显示“第无版”及入口技术 ID 常显也已修正；重建后的浏览器已复验公共地址、已完成标签和设备应用报告；首次发布与入口展示另有组件回归。

同一环境协议客户端（非 Android 真机）使用页面生成的接入资料成功 Enrollment。管理界面观察到：快照下载后仍未知 → 第 1 次发布已报告应用 → 发布第 2 次后待更新 → 第 2 次已报告应用。旧 generation Runtime 返回 428；最新 generation 的模型 SSE、TTS、multipart ASR、MCP initialize/list/call 均 200，回退应用报告返回 409。上游为合成协议服务，不证明供应商生成质量；隔离结果在 `.data/access-preview/runtime-evidence.json`，凭据文件不提交。9100 为本轮环境；9000 的旧预览进程已停止，避免旧后端与新前端混用。

最新验证（2026-09-18）：完整 `go test ./...`、`go vet ./...` 通过；Admin 21 文件 128 项测试、typecheck 和生产构建通过；Portal 8 文件 103 项测试、remote/local 构建、交付测试及真实 Hub/本地包 6 项浏览器回归通过。`TestPublicHTTPAndHTTPSControlLifecycle` 用真实 HTTP/受信测试证书的 HTTPS 连接验证登录、接入、Snapshot、应用报告和 Portal 换票/Cookie/CSRF；`TestRealtimeASRWebSocketAdmissionAndFrames` 验证 ws/wss，未关闭证书验证。报告跨 Session、过期、回退、错误 hash、不续期和下载不算应用均有回归；Hub 实际重启后的管理页面仍显示第 2 次发布的已应用报告。HTTPS 的证据是自动化协议连接测试，不冒充 HTTPS 浏览器人工操作或 Android 真机验收。

2026-09-18 曾生成一个 Portal 交接包，内含本仓库导出的 Core 资料，交接说明已消除 wss-only 的旧描述；HTTP 使用 ws，HTTPS 使用 wss，端口保留。当时按摘要清单逐文件比对，该机制现已移除，包内容由生成脚本从源文件复制。

Android 工作树已有其他维护方新增的 PlatformControlClient/Mapper/reportApplied，本轮只读核对，未修改 Android。七项上游交付审计完成；正式阶段 Freeze、供应商生成质量和 Android 真机验收不由本轮测试替代。

## 当前唯一契约

MEASIX 从未发布。Snapshot v4、Bridge v3、Enrollment formatVersion 1 和当前 HTTP `/v1` 是唯一实现目标；这些编号用于识别当前 wire contract，不表示兼容旧实现。Portal Session/Feed 只走 Core HTTP，不存在 local-read。旧 Snapshot、四项策略 DTO、策略收养、字段补值、双读/双写、增量数据库升级和历史命令别名均不维护。Gateway/Snapshot v5 仍是后续阶段目标。

当前数据库只有 `backend/migrations/202609120001_current.sql` 一份完整初始化 SQL。空库初始化后以 SQL SHA-256 作为 `schemaIdentity`；非当前开发数据库或配置直接删除重建，不转换、回填或旁路打开。当前 schema 的原子初始化、重复核对、完整性、备份恢复和失败不污染测试继续保留。

## A1–P3 当前结果

| Owner | 已完成内容 |
| --- | --- |
| architecture | Admin 配置工作台、Android 风格的分区/详情编辑、响应式行为、严格字段与 Review 基线写入产品和测试合同；数据库合同统一为当前 schema identity；Android 接线只要求当前配置初始化，不要求迁移旧配置 |
| Core wire/backend | Admin/Client OpenAPI 要求 Assistants、Starters 和五项策略全部显式存在；资源字段执行当前最小约束。Bootstrap 直接创建完整当前 Draft。Hub Preview 将保存的 Draft 与最新 immutable Release 比较，返回资源、Binding、Policy、Assistant、Starter 的权威 diff |
| Core Admin | 一个配置工作台覆盖 Overview、Models、Image Generation、TTS、ASR、MCP、Assistants、Policy；一级 Budget Templates 在 Users 与 Resources 之间，支持实时传播、显式能力覆盖和审计。Assistant 采用 collection → selected settings，包含 Basic、Instructions、Memory、Model & MCP、Starters。资源删除检查 defaults/Assistant/Binding 引用；Validation issue 可返回对应分区；脏 Draft 的刷新、离页和重新加载需要确认 |
| Core current-only cleanup | 删除旧的本地 diff 路径猜测、772 行重复 E2E、不可工作的 `freeze-gate` wrapper、旧 browser/schema 命令别名和迁移措辞；schema 工具只接受一份当前 SQL并拒绝增量历史 |
| Portal | 独立仓库只生成一套标准静态工作台；Core 默认分发该构建，或同源代理企业自有 HTTP/HTTPS 静态站点。Portal 不拥有 Managed 配置编辑，Core Admin 不复制 Portal 工作台 |

Android 集成导出只含客户端实际消费的内容：可执行 Client OpenAPI、Portal Bridge 契约、共享 fixtures、428 问题样例与接入说明；不内嵌架构文档正文，也不含 Core 测试源码。它由 `scripts/export-client-integration.mjs` 从本仓库源文件直接复制生成，不含摘要清单或独立校验器。

## 本轮验证

| 检查 | 结果 |
| --- | --- |
| Core backend | `go test ./... -count=1` 通过；`go vet ./...` 通过 |
| Core system smoke | `go test -tags=smoke ./test/system/scenarios/ -count=1 -timeout 5m` 通过；真实 Hub/Relay/SQLite + deterministic Adapter，覆盖同步 Image Generation 透明转发 |
| Core Admin | 30 个 Vitest 文件、178 项测试通过；`vue-tsc --noEmit`、E2E TypeScript 检查与 Quasar production build 通过 |
| Core browser | `node scripts/e2e-harness.mjs` 使用隔离 SQLite、真实 Hub/Relay、production SPA 与 Chromium，通过模板创建/指派/实时传播/覆盖清除、Image Generation 配置发布、五类 runtime traffic、usage/system 和 topology security |
| Current schema | 空库应用唯一一份 SQL 的 Go 测试通过（应用、业务读写、重复初始化、失败事务回滚） |
| Portal | 11 个 Vitest 文件、87 项测试、typecheck、production build、format check 通过；真实 Hub + production Portal Chromium 生命周期及 Image Generation/DashScope 组合筛选通过 |
| Android | `test assembleDebug lintDebug assembleRelease` 通过；Pixel 10 Pro Fold Android 17 上 `connectedDebugAndroidTest` 通过；另以 opt-in live 用例完成 Core Relay → DashScope → 安全下载 → GeneratedMediaStore 真链路 |

Windows 当前 Go 环境未启用 CGO，`go test -race` 在测试启动前被 Go 拒绝。历史 candidate system、Core/Admin 和浏览器证据不自动适用于本工作树最新候选；交付时必须记录本轮重跑结果。旧 Portal 双构建、本地包和 local-read 测试结果不再是当前证据。付费模型/语音供应商 qualification、独立 clean-source rebuild/replay 与最终 Freeze 仍需独立证明。Firecrawl 官方免密钥 MCP 的真实调用不能替代其他供应商证据。

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

Android 必须消费当前 exact 导出，保持 Snapshot v4、五项策略和 Bridge v3 一致，并完成真实 Platform source、Portal grant/session、远端 Relay 四 profile、428/刷新/退出和物理设备互操作证据。Core/Portal 的 Green 不能替代 Android 设备证据。

S0.3 的 Enterprise Tool Gateway、Snapshot v5、真实生产 supervisor/package 和后续 User Sync 不在本批实现范围，也没有预建空模块或兼容入口。

## 独立源码回放工具

已将重复的 runtime-only 回放替换为固定 Core/architecture 提交的独立检出、锁定依赖安装、契约再生成、生产构建摘要核对和完整测试回放。失败停止并保留日志，不覆盖候选或既有证据；正式清单单独生成，校验候选字节摘要、重建 facts、全部必需步骤及其日志摘要。真实临时 Git 仓库测试验证工作树修改/私有文件不会进入检出，失败命令不会执行后续步骤。工具功能测试通过不代表本工作树已完成正式 C7；当前尚无满足全部固定来源及真实四能力 qualification 条件的新候选，完整 clean-source gate 未执行。具体命令见 [release](release.md)。

测试上游身份摘要现覆盖完整 Go adapter/client 目录和浏览器测试上游源码，修复新增协议文件变化不影响旧三文件摘要的问题。工具测试共 15 项通过；检出测试额外验证源仓库 HEAD 已前进时仍取指定旧提交。删除约 600 行重复回放流程，复用完整浏览器 harness；本轮没有改变客户端 wire，也没有修改 Android。

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
| 可独立交给 Android 的协议与静态包 | 接入说明的六步实施清单；Portal 交接包内含本仓库导出的 Client 资料，包内容由 `scripts/export-client-integration.mjs` 从本仓库源文件复制 | 包不是冻结提交，设备验收仍由 Android 完成 |
| 当前唯一版本、清除无意义旁路 | 单一初始化 SQL、严格当前 Snapshot、删除旧 diff/重复流程/误导测试命名 | 两份误生成的 `backend/backend/internal/wire/{adminapi,clientapi}/*.gen.go` 已由用户删除，重新检查均不存在；正常生成源码保留 |

下一步交接应直接使用 [Android 实施清单](android-platform-integration.md#验证职责与交接清单)，而不是让 Android 猜测资源协议或上游密钥。Android 维护方正在独立接入 Platform source，应使用当前工作区实现核对交接，不能把本轮上游交付说成 Android 已接通。正式阶段 Freeze 与设备验收继续按原门禁执行。

本次本地交付收尾：用户删除误生成文件后，确认两个错误路径均不存在、两个正常 wire 文件仍存在；重新执行 `go test ./... -count=1`、`go vet ./...`、14 项 Node 工具测试、72 文件 Core 独立导出校验和 `git diff --check` 全部通过。Android 工作树保持干净。架构/Core/Portal 本轮实现、管理员审查、协议扩展与交接资料已交付；没有提交、发布或宣布正式 S0.1/S0.2 Freeze。付费供应商测试按用户确认的官方协议加严格合成上游方式完成，真实 Firecrawl 证据单列；Android 设备与公网部署验收继续由后续接线承担。

## Android 接入可用性复核

重新核对路线图及 Foundation/Realm/Android authority 后，补充明确“正式阶段 Exit 顺序不禁止当前合同提前联调”：v4 的四模型、四 TTS、三 ASR、Direct MCP 可以现在适配，不等待 Gateway，不制造空 Gateway readiness 或 v4/v5 双轨。S0.4 的正式冻结条件保持不变。当前定位是 S0.1/S0.2 上游候选已具备，Android 真实平台 source/设备闭环仍待维护方完成；不是全部 S0.2/S0.4 已通过。

接入说明新增个人空间体验边界与逐项阻碍处理：手机可信 HTTPS、真实上游配置、五项 allowLocal 策略、辅助模型选择、认证/428 恢复、10 MiB 请求/连接限制。现场读取 generation 9 确认四模型/四 TTS/三 ASR、一个助手、两个入口、两个 MCP，五项 allowLocal 均为 false；这是隔离测试配置，不是可忽略的实际产品默认。新增四协议图片输入与历史消息逐字节转发测试通过，连同供应商、工具和 WebSocket 相关回归通过；它证明 Relay 不剥离图片，不证明模型视觉能力或设备编码。

资料同步后另生成过一个 Portal 交接包，内含本仓库导出的 Core 资料。Portal 页面/Bridge 无变化，沿用同一已验证生产构建；本轮变化是架构说明、接入指南及新增 Relay 回归。Android 未修改。
