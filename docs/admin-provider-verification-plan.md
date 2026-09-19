# 管理台审查与提供商接入验证

状态日期：2026-09-18。本文记录本轮范围、可复核证据与边界；阶段状态统一见 [当前实现状态](s0-execution-progress.md)，产品和协议权威属于 architecture。Android 仓库只读。

## 交付范围与验证方式

用户明确本批完成 S0.1/S0.2 能力及本次协议扩展；Gateway 留在 S0.3。当前覆盖身份/接入/会话、四种模型、四种 TTS、四种 ASR、Direct MCP、五项策略、助手/记忆种子/常用入口、企业动态、发布、用量与管理界面。MEASIX 从未发布，只维护当前契约和一份数据库初始化结构，不建立旧格式转换、默认补值或旁路。

最初使用合成凭据和严格本地 HTTP/WebSocket 上游验证真实 Admin、Hub、Relay、SQLite。随后用户提供四家供应商密钥，本机直连及隔离 Core Relay 的实际结果见[真实供应商接入记录](real-supplier-integration.md)。合成验证、真实供应商响应、当前环境发布与 Android 设备消费分别验收，不能相互替代。

## 管理员实际操作

在隔离 `.data/admin-preview/hub.db` 和生产 SPA 中，从空库完成创建用户/一次性接入资料、创建凭据/上游、应用连接、配置资源与五项策略、助手/记忆种子/两个入口、保存/验证/预览/审查/发布，以及企业动态发布/撤回/再发布。随后逐次发布新增协议，最终为 generation 9 / draft revision 12。

实际使用全部八个入口：Overview、Users、Resources、Upstreams、Releases、Enterprise Updates、Usage、System。修复和复核包括：

- 配置按资源名称与任务分区组织；技术编号、路由和高级参数折叠。默认资源、助手引用、记忆和入口在最终快照中可审查；失效引用可定位修正。
- 保存后的草稿由 Hub 与不可变 Release 比较；删除重复的前端 diff 猜测。没有变更不产生空发布；发布状态只呈现真实 Activation state，不虚构阶段。
- Secret 元数据支持刷新后复用；上游应用使用页内确认、防重复提交。HEAD 探测只说明可达性和 HTTP 状态，不宣称模型、流式或工具能力已经验证。
- 用户设备以短标识、应用版本和本地时间辨认，完整编号留在详情。资源与策略标签本地化；刷新操作有可访问名称。八个入口的窄屏布局已审查。
- System 分别显示 currentActivation 和 lastActivation；没有在途操作时明确提示，最近完成结果不被在途状态覆盖。显示真实 Relay 构建标识；未知观察不伪装为零或健康，上游应用状态不冒充连通性。
- Usage 使用本地日期选择器、历史发布资源名称；技术编号筛选默认折叠。详情可直接筛选同一资源，实际 Firecrawl 结果为 4 条。名称从请求所属 generation 的快照读取，不能由当前草稿改写历史。
- 连续切换筛选时清空旧数据并忽略过时响应，旧查询错误或分页不会污染新条件结果；三个乱序响应测试覆盖此边界。
- 完整度请求数由后端同筛选/读事务统计，每条请求一次；真实预览库为 25 未知。单条详情不再借用查询汇总成本冒充该请求成本。

## 协议与发布证据

| 能力 | 管理界面与公开客户端链路 | 严格协议/运行证据 | 边界 |
| --- | --- | --- | --- |
| Chat Completions、Responses、Gemini、Claude | 四种模型经 UI 配置/发布；公共 Enrollment → Managed State → Snapshot → Relay 均返回文本 SSE；Gemini/Claude 同时返回工具调用 | `provider_protocol_test.go` 校验路径、凭据替换及供应商 header；`model_tool_roundtrip_test.go` 校验两轮工具 body、ID/reasoning/signature 保留、429/Retry-After；既有取消/断流回归通过 | 本机另已运行真实 DeepSeek Flash 文本、工具、图片与 Relay profile；CLIProxyAPI 部署未测，Android 设备往返另验 |
| OpenAI、MiMo、Gemini TTS | generation 7 公开客户端链路返回对应二进制、SSE 音频增量、JSON 内联 PCM | MiMo 用 api-key；Gemini 用 x-goog-api-key；平台 Authorization 被移除。严格上游拒绝 MiMo 错误 Bearer，实测 401。新 generation 3 经公共入口调用真实 MiMo TTS，WAV 与流式 PCM 成功 | 原合成测试载荷不证明播放；真实生成与设备播放仍是两项不同验收 |
| 系统 TTS | generation 4 起下发 speechRate=1.2、pitch=0.9；allowLocalTts=false 时仍可作为企业默认 | compiler/共享样例验证无 runtimePath、无上游绑定与云端专属字段 | Android 系统引擎播放由设备验收 |
| HTTP ASR、OpenAI/DashScope 实时 ASR | HTTP 文件转写及 generation 5 两种实时资源经公共客户端调用；本轮补 DashScope HTTP JSON ASR 的新协议、Admin 编辑和 Android 共享样例 | 严格会话验证配置→PCM append→增量/完整转写；DashScope finish/finished。Relay 覆盖握手、双向帧、凭据、query、generation、取消、idle/overall timeout、累计输入限额及 101 计量。百炼 HTTP ASR 已直连返回真实转写，公共入口实验结果另见真实供应商记录 | 合成实时转写不等于付费实时服务或设备麦克风测试；新 HTTP profile 还需 Android 原生适配 |
| Direct MCP | 初始本地工具与 generation 9 官方 Firecrawl 均通过 UI 发布及助手引用 | `mcp_session_test.go` 验证 initialize/202/list/call SSE、会话/版本头、GET 405、DELETE 204、过期 404 | 通用凭据注入测试与 Firecrawl 真实免密钥证据分开；不经 Gateway |
| Firecrawl hosted MCP | HTTPS `https://mcp.firecrawl.dev/v2/mcp`，Direct MCP，企业助手引用 | 初次免密钥验证后，又使用用户 Bearer 凭据完成 initialize/通知/list/真实 scrape 与隔离 Relay profile；详情见真实供应商记录 | 工具目录随账户权限变化，不能由一次结果推定所有工具均可用 |

公开客户端验证脚本位于忽略的 `.data/admin-preview/verify-models.ps1`、`verify-tts.ps1`、`verify-asr.ps1`、`verify-firecrawl.ps1`；不输出/持久化访问令牌。可复用断言保留在 Relay、adapter、compiler 与共享协议测试中，不以这些本机脚本替代正式测试。

## 官方协议来源

- DeepSeek：[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)、[工具调用](https://api-docs.deepseek.com/guides/tool_calls/)。不能推定 DeepSeek 支持 Responses。
- CLIProxyAPI：[官方配置](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)。两组 OpenAI 接口分别验证，模型和能力由实际部署决定。
- OpenAI：[Function calling](https://developers.openai.com/api/docs/guides/function-calling)、[Realtime transcription](https://developers.openai.com/zh-Hans/api/docs/guides/realtime-transcription)。
- MiMo：[TTS 指南](https://mimo.mi.com/docs/zh-CN/quick-start/usage-guide/audio/speech-synthesis-v2.5)。Chat Completions 音频请求不套用 OpenAI Speech。
- Gemini：[语音生成](https://ai.google.dev/gemini-api/docs/speech-generation)、[thought signatures](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures)。
- Claude：[工具结果处理](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)。
- DashScope：[实时客户端事件](https://www.alibabacloud.com/help/en/model-studio/qwen-asr-realtime-client-events)。
- Firecrawl：[官方 MCP](https://github.com/firecrawl/firecrawl-docs/blob/main/mcp-server.mdx)、[免密钥接入](https://raw.githubusercontent.com/firecrawl/firecrawl-docs/main/mcp-server/keyless.mdx)。官方免密钥与企业 Bearer 配置分别表达，不将密钥放入 URL。
- MCP：[Streamable HTTP](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)。本轮固定当前合同，没有旧 SSE 兼容分支。

## 自动化证据与交接

最新 Go 全量测试、vet、candidate system（真实 Hub/Relay/SQLite）、Admin 20 文件/116 测试、类型检查、生产构建及五阶段浏览器 harness 均通过。Portal 102 项测试、2 项交付测试、remote/local 构建通过；浏览器回归结果见 living status。最新筛选修复后的完整 harness 再次通过；其中一次隔离 SPA 代理就绪检查失败，相同端口/worker 独立探测返回 200，重新建立隔离环境后五阶段通过，未将失败记作通过。完整生成链检查了 281 个文件，wire/fixtures/导出无漂移；go mod tidy 将实际使用的 WebSocket 库归为直接依赖，再次 tidy 稳定。

运行中的统一预览入口为 Caddy 2.11.4，9000；Hub 为 control-hub-diagnostics.exe，9004/9001；Relay 为 runtime-relay-asr.exe，9002/9003。此前脚本直连 Relay 的证据仅证明组件链路；现已改为从 9000 的 Discovery 解析 Client/Runtime 基址，重新通过四模型、三云端 TTS、两种实时 ASR 和真实 Firecrawl initialize/通知/list/scrape 验证。系统 TTS 核对下发参数，不冒充设备播音。新 Firecrawl requestId 为 `req_811800c1-f28f-4f75-9520-b9dea302f6d5`。具体接线见 [operations](operations.md#one-public-origin)。

[Android 接入说明](android-platform-integration.md) 按平台 source、原子快照、Runtime owner、四模型/四 TTS/三 ASR、Direct MCP、Portal 生命周期给出实施顺序及共享样例。Android 当前个人/本地能力不等于平台接入；本轮未修改或编译 Android，也未提供设备消费完成声明。

正式阶段 Freeze 仍需对应门禁与精确来源/构建/证据链，包含独立 clean-source rebuild/replay。当前本机脏工作树和 deterministic 上游通过不替代该结论。两份错误嵌套生成文件 `backend/backend/internal/wire/{adminapi,clientapi}/*.gen.go` 已由用户手工删除，现场核验均不存在，原清理阻碍已解除。
