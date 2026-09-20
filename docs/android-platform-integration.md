# Android 真实平台接入说明

本说明面向 Android 维护方。当前唯一组合是 Discovery protocolVersion="1"、Snapshot v4 与 Portal Bridge v3；Enrollment 资料 formatVersion=1 独立。MEASIX 从未发布，不做旧 Snapshot、旧数据库或配置转换。本说明约定目标接线和验收，不以某个 Android 历史版本的缺口代替当前源码审查；上游测试不代表设备联调已完成。

## 权威与资料入口

语义权威在 `topabomb/measix-architecture`，本包不复制其正文，只按文档名与章节引用：

- **Control Protocol**：§8 认证、§10 配置、§11 Runtime。协议语义有异议时以它为准。
- **Enterprise Realm / Experience Contract**：企业配置与用户数据边界。
- **阶段阅读清单 `measix-stage-document-index.md`**：S0.2 范围与下游测试入口。

本包内的可执行与样例资料：

- [Android 可执行 OpenAPI](../api/generated/android/client-control.openapi.yaml)：已展开外部依赖；HTTP 请求、响应、enum 的唯一可执行结构来源。
- [共享接入资料](../api/fixtures/client-integration/cases.json)、[HTTP 时序示例](../api/fixtures/client-integration/http-examples.json)、[完整 Snapshot](../api/fixtures/client-integration/snapshot-v4.json)、[全部禁止策略](../api/fixtures/client-integration/snapshot-v4-denied.json)、[语义反例](../api/fixtures/client-integration/reference-cases.json)。
- [接入材料正反例](../api/fixtures/enrollment/cases.json)、[原生 Bridge 正反例](../api/fixtures/portal/native-vectors.json)、[Feed 正反例](../api/fixtures/portal/feed-vectors.json)。

样例由 `cmd/generate-client-fixtures` 调用真实 Snapshot compiler 生成，输入是公开的资源投影配方，bindings 为空，不是可直接发布的运营草稿（真正发布需管理员配置私有 Upstream/Secret/Binding）。合成令牌、接入码、时间和 ID 只用于测试，不能用于服务器登录。Canonical hash 由 core 当前 `capability.HashSnapshot` 复算验证；客户端按 Control Protocol §10.13 校验 ETag/body.snapshotHash 一致性，不另造 JSON canonicalization。文件 SHA-256 与 Snapshot hash 是不同概念。

独立包 `api/generated/android/integration` 只含本说明、可执行 OpenAPI 与客户端消费的 schema/fixtures；不含架构文档正文，也不含 Core 测试源码。它由 `scripts/export-client-integration.mjs` 从本仓库源文件直接复制生成，可整体取用；需要追溯来源时看本仓库，不要直接编辑包内文件。

## Android 消费模型映射

### 现在能否开始，以及与个人空间的关系

可以开始当前 v4 的真实平台适配与联调，依据 Foundation Contract §3；无需等待 Gateway 或把正式 S0.4 Freeze 当成 Runtime 开关。当前 Hub/Relay 没有“尚未 Freeze 禁止调用”的分支。正式阶段通过仍须完整门禁，不能用本地联调替代。

企业域复用 Android 现有聊天、流式输出、推理展示、工具循环、图片理解、朗读、录音转写、助手、对话和本域记忆 owner。平台改变资源来源、认证、准入和归属，不应另造一个删减的聊天运行器。图片理解须同时满足资源声明 IMAGE、上游模型支持和原生编码；平台不提供图片生成或 Embedding 资源，亦不提供云端对话/附件同步。个人原配置和数据不被企业配置覆盖。

| 可能阻止使用的条件 | 本轮明确的处理方式 |
| --- | --- |
| 原生仍拒绝 PLATFORM_ENROLLMENT | 实现并修复唯一 Platform source；不能靠 Portal 包或手机端模拟企业替代 |
| 手机不能访问开发电脑的 127.0.0.1 | 配置手机可达的 HTTP 或 HTTPS 统一公共入口（域名或 IP 均可），Hub PublicOrigin 保持相同；Discovery/Client/Runtime/Portal 都从该 origin 访问。具体 ingress 配置由 Core operations 文档维护 |
| 当前预览上游是合成服务 | 它只证明协议和转发；真实使用须在 Admin 配置实际供应商/CLIProxyAPI 地址、模型、凭据并应用、发布。客户端不接收企业密钥，不能把合成回复当成真实生成 |
| 五项 allowLocal* 为 false | 这是禁止企业域使用用户自带配置，不是禁用企业下发的资源。需要混用时由管理员启用对应策略并发布，不能由客户端越权绕过 |
| 辅助功能没有选中的可用资源 | title/fast/compress 等槽位沿用本域选择规则；不要引用个人域中已被策略禁止的资源，也不要要求 Snapshot 存在未定义的额外默认字段。选择合适的可用企业模型；图片生成没有企业模型时明确不可用 |
| 401、403、428 或配置尚未发布 | 按认证/撤销/原子同步语义恢复并给出可理解状态。仅有明确未转发保证的请求可重新准入；不能自动重发已执行工具。管理员先发布一个有效 Release 并确认 Relay 已应用 |
| 大图片、长录音、供应商限流或超时 | 当前 Hub 编译 Runtime 请求上限为 10 MiB；HTTP 按请求体、WebSocket 按连接累计客户端帧字节计数，base64/JSON 也计入。客户端应在编码后控制大小，采用有界录音会话，超限明确提示重新录制/压缩；不得静默截断或无限重试。Upstream 路由超时需按实际供应商配置，429/供应商容量限制不能靠协议适配消除 |

Direct MCP 的企业共享凭据或 NONE 模式现在即可使用；企业动态作为模型工具、Gateway 发现/调用、托管 Agent 与云端同步分别按后续路线图实施，不是当前四模型/语音/Direct MCP 的运行前置。设备验收应覆盖“全部 allowLocal* 关闭仍可使用企业下发资源”和“按策略允许混用用户资源”两组，不以关闭安全检查换取可用性。

模型协议当前明确包含 `OPENAI_CHAT_COMPLETIONS` 与 `OPENAI_RESPONSES`。Responses 对照共享 `snapshot-v4-responses.json` 和 `cases.json` 的 `v4-responses` 用例消费；从 Provider 的 `clientProtocol` 选择 Android 的 Responses 编码/解码路径，不从 Relay 域名或模型名称推断。完整接口路径取 Model.runtimePath，不能再额外追加 `/responses`。当前执行约定为完整 input、stream=true、store=false，不依赖 previous_response_id 或服务器对话状态；工具结果与必要 reasoning items 由客户端继续带入 input。Core 不翻译两组请求。Android 私有本地包中的 `OPENAI_RESPONSES` 枚举存在不等于平台 Snapshot 已接通，需维护方验证实际平台映射与对话执行。

另外两种当前模型协议 `GOOGLE_GENERATE_CONTENT`、`ANTHROPIC_MESSAGES` 分别使用 `snapshot-v4-gemini.json`、`snapshot-v4-claude.json`。复用 Android 原生 provider 编码器，但把地址和认证接到平台 Runtime owner：Gemini 使用完整 runtimePath 并追加 `alt=sse`；Claude 保留 `anthropic-version` 和 `max_tokens`。工具后续请求须保留调用与结果的配对、适用 reasoning items 和 Gemini thoughtSignature。不能在 Relay 中解析、补全或重写这些供应商字段。

以下 Kotlin 名称只用于定位当前实现，不改变 wire。平台 source 应有独立适配层，复用现有 ConfigurationResolver/执行准入；不存在并行的手机端示例企业 source。

| 平台字段/事实 | 当前 Android 模型处理 |
| --- | --- |
| deploymentId + userId | `deploymentId` 是企业身份，`deploymentId + userId` 是企业用户数据范围。HTTP/HTTPS origin 只由当前 Enterprise Session 作为可变连接参数持有，不参与 Realm、配置 scope、资源引用或本机数据归属。修改地址时必须在候选 origin 上核对同一 deploymentId 和当前 Session 的 userId/deviceId/sessionId；不同 deploymentId 明确拒绝 |
| userId/deviceId/sessionId | 用户域归属与认证会话分别保存；重登可以同 user/device，但必须采用服务端新 Session，不复活旧文档或旧任务 |
| managedGeneration / releaseId / snapshotHash | generation 映射 EnterpriseConfiguration.generation；同时保存用于一致性验证的 release/hash，整个候选验证后原子替换 |
| Provider.providerId/displayName/clientProtocol/enabled | 适配层保留 Provider 与协议元信息；现有 EnterpriseModel 没有完整 Provider 表，不能丢失后猜测协议。禁用 Provider 下的资源不可执行 |
| Model.modelId / upstreamModelKey | 前者是稳定平台资源 ID，映射 EnterpriseModel.id；后者映射其请求模型名 modelId，写到请求 body 的 model。不能互换 |
| Model.displayName/modalities/capabilities | name 与显式 enum 映射。S0.2 为 CHAT；不映射成图片生成/附件等未声明 profile。未知 enum 拒绝候选，不能默认为普通文本模型 |
| Provider.clientProtocol + Model.runtimePath | 保存在平台运行适配配置中，决定公开 Relay URL 与请求 profile；不塞入本地私有 binding，也不向用户存储注入企业上游地址/密钥 |
| TTS.ttsId/displayName/clientProtocol | 保留企业资源身份，按 Control Protocol §10.5 显式分派四种语音执行方式。现有企业 OpenAI 专用通道需扩展，不能按模型名猜协议 |
| 云端 TTS.upstreamModelKey/voice/runtimePath/voiceDesignPrompt | 分别保留模型、预置音色、Relay 路径与 MiMo 描述。音色设计无 voice；Gemini JSON 内联 PCM，MiMo SSE 音频增量，OpenAI 二进制音频，各用对应编码/解码 |
| SYSTEM_TTS.speechRate/pitch | 仅设备执行，无上游绑定、Runtime URL 或企业服务器密钥。仍为企业资源，可在 allowLocalTts=false 时作为 defaultTtsId；缺少可用设备引擎须明确报错，不切换云端 |
| ASR.asrId/displayName/upstreamModelKey/language/clientProtocol | 保留资源身份、模型及可选语言；按 clientProtocol 分派 OpenAI multipart、DashScope HTTP JSON、OpenAI Realtime 或 DashScope Realtime。实时参数按当前协议映射，不把 WebSocket 转成文件上传 |
| MCP.mcpServerId/displayName/enabled/authOwnership | EnterpriseMcpResource.id/name/enabled；适配层保留 MCP_STREAMABLE_HTTP、runtimePath 和实际 authOwnership（ENTERPRISE_MANAGED 或 NONE）；均使用 Relay，无 OAuth/上游凭据下发 |
| Assistant.assistantDefinitionId/displayName/description/modelId/systemPrompt/mcpServerIds/enabled | EnterpriseAssistant 对应字段；缺省 description 可展示为空字符串；引用的 modelId 是平台稳定 ID，不是请求模型名 |
| Assistant.memorySeed[] | 按作者顺序转换为只读 Seed。内部 ID 从 deploymentId、助手 ID、generation、索引确定；空数组有效，条目不得为空白。不按内容去重、不建立可变 Assistant Memory 副本，配置替换时整体换代 |
| Starter.starterId/assistantDefinitionId/title/prompt/description/sortOrder/enabled | EnterpriseStarter 对应字段；仅启用且助手有效的入口可操作。展示按 sortOrder、starterId 排序。点击只进入原生输入草稿，由用户发送，不新增 Portal 聊天写入 Bridge |
| policy 五项 allowLocal* | EnterprisePolicy 五项必填 Boolean；缺失/null/错误类型均拒绝。只控制本企业域内用户原配置准入，不复制用户定义、端点和密钥 |
| defaultModelId/defaultTtsId/defaultAsrId/defaultAssistantId | defaults.chatModelId/ttsId/asrId/assistantId；显式无效引用不回退首项。没有用户已选助手时采用 defaultAssistantId；用户已选助手失效时呈现选择与修复入口，不静默改选默认助手。未提供的默认值保持未指定，由既有本域选择规则处理 |
| defaults.fastModelId/titleModelId/imageGenerationModelId/attachmentInspectionModelId/suggestionModelId/compressModelId | v4 无对应的企业强制字段。保留本域偏好和功能已有选择规则，不把 defaultModelId 批量写入所有槽位 |
| allowAsSubAssistant/allowedSubAssistantIds、gateways | v4 不下发企业子助手关系或 Gateway；平台适配输出 false/空集合。用户自有子助手仍按现有五项准入和执行权限处理，不扩展 wire |

## 认证、同步与恢复时序

1. 扫码或粘贴只解析共享 EnrollmentMaterial；明确 sourceKind，再验证 HTTP/HTTPS 来源与 Discovery。校验 product、protocolVersion、deploymentId 和当前支持版本，未知来源不得借本地域配置绕过。
2. `POST /api/client/v1/enrollments/exchange` 使用一次性 code、稳定 installationId、deviceName、appVersion、ANDROID；成功 201。原子保存整组 access/refresh、各到期时间、身份。失败不建立半个企业域。
3. access token 只用于 Client/Runtime Bearer；Portal JS 不可读取。`GET bootstrap` 的 session.expiresAt 是会话 idle 到期时间，不能替代 accessTokenExpiresAt。七天 idle 只由有效刷新续期，普通查询不续期。
4. 没有首个 Release 时 activeManagedGeneration=0，保持待配置且 Runtime 禁止；不请求 Snapshot 0、不因 READY 字样而执行。首次 Bootstrap 不带已应用 generation，因此有发布配置时也需要同步。
5. `GET managed/state` 可带 `X-Measix-Applied-Managed-Generation`。下载目标 generation 的 Snapshot，验证部署/版本/generation、ETag 与 body.snapshotHash 一致性、必填策略、资源 enum 与引用闭包后原子应用，之后才通过 `PUT /api/client/v1/managed/applied` 报告 `{managedGeneration,snapshotHash}`，使用当前母 Session 的 Bearer；成功返回 204。状态查询 header、下载和 304 不记录回执。报告失败不回滚配置或重放业务，下次同步检查重报当前已应用值；409 表示同 Session 倒退报告，422 表示发布/hash 不匹配。保留最后完整状态用于展示，但不能绕过新状态的执行阻止。
6. Snapshot HTTP 200 返回 JSON 与带引号的 ETag；If-None-Match 命中返回无 body 的 304。仅当同一来源、身份及 generation 的已验证缓存存在时才能复用。Portal 数据使用独立的同源 Core HTTP Session/Feed，不经过原生 Bridge 伪造 HTTP 语义。
7. 每个 Session 的 refresh 串行化。先持久化本次 Idempotency-Key；不确定响应只用同一旧 refreshToken + 同一 key 重试。服务端短恢复窗内返回完全相同的轮换结果；原子提交新令牌后才开启下一次刷新。旧 token + 新 key / 新 token + 旧 key 为 409 refresh_conflict。恢复窗已过按明确失败重新接入，不循环重试。
8. Runtime 请求冻结 authority/session/generation/resource；`428 managed_snapshot_required` 且 forwarded=false 表示未转发，进入配置同步与重新准入，不能绕过 generation barrier 或盲重放已经发生 I/O 的业务。
9. Client logout 是 `POST sessions/logout`，JSON body 携带当前 refreshToken；不是 access-token-only 请求。成功 204。Client/Refresh/Runtime 对已知 User disabled、Device revoked、Session revoked 分别返回 403 `user_disabled`、`device_revoked`、`session_revoked`，正确 ETag 也不能绕过授权；缺失认证为 401 `unauthenticated`，坏 credential/JWT 仍是 401 类认证失败。Android 将三种 403 都交给同一授权撤销生命周期，但保留原 code 用于明确诊断。
10. 原生确认取消保留原状态；确认后由持久 owner 完成退出。先撤销旧文档交付、取消/等待原生采集与写入，再清理 Realm/Portal 站点数据和媒体。站点清理失败可重试，完成前不得展示新 Realm。旧接收器、旧媒体句柄、旧请求不能写回新页面。

## Runtime 请求示例

从可信 origin 拼接 Discovery.runtimeApiBase + `/resources/` + **稳定资源 ID** + 该资源 runtimePath。示例 origin 为 `https://platform.example.invalid`。不能把 upstreamModelKey 放在 URL 资源 ID 位置，不直接请求 Upstream，不用 Portal Cookie 调 Runtime。

每个请求带 `Authorization: Bearer <accessToken>`、`X-Measix-Managed-Generation: 42`、`X-Measix-Interaction-Id: <本次合法 interaction ID>`。后两个值由原生执行上下文冻结；Managed State 查询使用的 Applied header 不能替代 Runtime header。

| profile | 示例 POST URL 后缀 | 请求与响应 |
| --- | --- | --- |
| OPENAI_CHAT_COMPLETIONS | `/runtime/v1/resources/mdl_bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb/v1/chat/completions` | JSON `{"model":"gpt-4o","messages":[{"role":"user","content":"你好"}],"stream":true,"stream_options":{"include_usage":true}}`；SSE 按事件读取，取消关闭响应流。非流式使用 stream=false。tools/tool_calls 和图片内容依照声明 profile，由客户端 SDK 构造；Relay 不翻译 body |
| OPENAI_RESPONSES | 同一资源 URL 规则，runtimePath 通常为 `/v1/responses` | POST JSON + SSE；完整 input、store=false；回传 function_call_output.call_id 及相关 reasoning items，终止事件 response.completed |
| GOOGLE_GENERATE_CONTENT | runtimePath 通常为 `/v1beta/models/{upstreamModelKey}:streamGenerateContent`，追加 `?alt=sse` | contents/parts、functionCall/functionResponse；保留模型返回的调用 ID 和 thoughtSignature；解析 candidates |
| ANTHROPIC_MESSAGES | runtimePath 通常为 `/v1/messages` | messages、独立 system、max_tokens、stream=true；tool_use/tool_result 配对；anthropic-version 公开协议头；解析命名 SSE 直至 message_stop |
| OPENAI_AUDIO_SPEECH | `/runtime/v1/resources/tts_cccccccc-cccc-4ccc-8ccc-cccccccccccc/v1/audio/speech` | JSON `{"model":"tts-1","voice":"alloy","input":"你好"}`；读取二进制音频，不当 JSON/SSE 解析 |
| GEMINI_GENERATE_CONTENT_TTS | runtimePath 中的 `:generateContent` 路径 | responseModalities=AUDIO，speechConfig 设置下发 voice；解码 inlineData 中的 PCM 和采样率 |
| MIMO_CHAT_COMPLETIONS_TTS | runtimePath 通常为 `/v1/chat/completions` | 按当前 Android MiMo 编码器构造 audio/messages，分别消费标准 voice 或 voiceDesignPrompt；读取 SSE 音频增量 |
| SYSTEM_TTS | 无 Runtime URL | 设备系统引擎执行 speechRate/pitch；保持企业资源归属，allowLocalTts=false 不禁用这个企业定义 |
| OPENAI_AUDIO_TRANSCRIPTIONS | `/runtime/v1/resources/asr_dddddddd-dddd-4ddd-8ddd-dddddddddddd/v1/audio/transcriptions` | multipart/form-data：model=whisper-1、language=en、file=<录音>；由 HTTP 库生成 boundary。解析 transcription 响应；不是 WebSocket ASR |
| DASHSCOPE_HTTP_ASR | 资源自己的 `/api/v1/services/aigc/multimodal-generation/generation` runtimePath | POST application/json：`model=upstreamModelKey`，`input.messages` 最后一条用户消息带 `input_audio.data=data:audio/wav;base64,...`，`parameters.format=wav`（MP3 同理）；从 `output.text` 读取文字。可选语言放 `parameters.language_hints=[language]`。不发送 multipart、实时参数或 WebSocket 握手 |
| OPENAI_REALTIME_TRANSCRIPTION / DASHSCOPE_REALTIME_ASR | 同一资源路径规则，HTTP 平台使用 `ws`、HTTPS 平台使用 `wss`，保留端口；分别追加 `intent=transcription` / 编码后的 `model` query | GET WebSocket 握手保留三项平台头。按下发 sampleRate/vadThreshold/silenceDuration 及协议专属 prefixPadding/prompt 建立会话；取消或退出关闭连接。完整事件与字段见 Control Protocol §10.6，不能改成文件上传 |
| MCP_STREAMABLE_HTTP | `/runtime/v1/resources/mcp_eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee/mcp/v1` | 标准 MCP Streamable HTTP 会话：initialize → initialized → tools/list → tools/call；Accept 同时允许 application/json 和 text/event-stream。沿用当前 MCP 客户端维护返回的会话/协议 header，不自创固定服务端 Session，不经 Gateway |

Direct MCP 的平台路由允许 `POST`、`GET`、`DELETE` 到 Snapshot 给出的同一 `runtimePath`；Android 保留上游会话 header。GET 事件流是服务端可选能力，上游返回 405 应按“不提供该流”处理；不应收到 Core 的 `ROUTE_POLICY_DENIED` 403。会话终止时的 DELETE 结果依上游会话状态处理，不把无 Session ID 时的 400 误判为平台拦截。

`DASHSCOPE_HTTP_ASR` 的协议客户端请求与按当前 Android 编码构造的有效 WAV 均已返回 200，但**设备录音是否普遍可用尚未确认**：一次模拟器录音经 Relay 转发后收到上游 400（同结构的纯静音 PCM 也能复现 400），这不是 Core 拒绝。设备维护方需核查录音的 RMS、WAV 头、实际采样率/声道/时长与非零样本数，用有声录音复测后再把设备级识别计为通过；不能仅凭平台编码单测或已知有效样本 200 推断所有设备录音可用。

完整 428 body 使用 [现行 Problem 样例](../api/fixtures/problem/managed-snapshot-required.json)。这些是请求构造示例，不声称合成 profile 已取得真实供应商资格认证。

## 验证职责与交接清单

共享样例覆盖当前全部能力：`snapshot-v4.json`（基础）、`snapshot-v4-responses/gemini/claude.json`（三种模型协议）、`snapshot-v4-speech.json`（四种 TTS，默认系统朗读、个人 TTS 准入关闭）、`snapshot-v4-asr.json`（四种 ASR）、`snapshot-v4-denied.json`（全部策略禁止）。请求构造示例见 `http-examples.json` 与 `runtime-examples.json`。Android 应对全部样例补候选校验、映射和执行分派测试，再做设备消费验收；上游资料更新本身不表示原生接线已完成。

上游协议证据入口在 Core 源码仓库，不在本包内：`contract` 复算 Snapshot hash；`httpapi/client_integration_test.go` 用真实身份/SQLite/HTTP 验证 pending、应用后状态、200/304 与撤销；`identity` 验证轮换幂等、并发与退出隔离；`relay` 的 `model_tool_roundtrip_test.go`、`provider_protocol_test.go`、`mcp_session_test.go`、`websocket_test.go` 覆盖四协议工具往返与 429、供应商认证边界、完整 MCP 会话以及握手/帧/超时/取消/准入。这些是上游证据，不替代 Android 设备验收；供应商付费账户权限与真实生成质量另行验证。

Android 维护方按以下顺序实施，本批不修改 Android 仓库：

1. 独立 Platform source：解析完整接入材料、Discovery、Enrollment、安全令牌存储与单一刷新 owner；与本地来源明确分派，不能把个人或本地企业包的可用性当成平台接入已完成。
2. 原子消费当前 Snapshot：完整 Provider/Model、四种 TTS、四种 ASR、MCP、五项策略、Assistant/Memory/Starter 映射；未知值、非法条件字段和失效引用拒绝整个候选，不改写成另一协议。切换企业/个人时资源和任务隔离。
3. 统一平台 Runtime owner：从 Discovery 和稳定资源 ID 组装 URL；HTTP 与 WebSocket 都使用平台令牌、generation 和 interaction 上下文。供应商密钥仅由 Relay 注入，客户端不添加或持久化这些密钥。处理 typed 428、撤销、刷新和取消。
4. 四模型分别验收文本流及工具往返；四 TTS 验收播放、停止和切换；三 ASR 验收录音、转写和取消。SYSTEM_TTS 在个人 TTS 禁止时仍能作为企业默认服务运行。工具结果保留 ID/签名，不重发已执行工具。
5. Direct MCP 使用企业资源和助手 mcpServerIds；初始化、通知、发现、调用、会话头与 GET/DELETE 按当前协议。Firecrawl 可先使用官方免密钥 `/v2/mcp`、authOwnership=NONE；企业 Bearer 方案由服务端配置。Gateway 不在本批范围。
6. 复用现有 Portal/退出 owner 完成远端站点、媒体、旧文档接收器清理。进程死亡、WebView、相机/麦克风和播放体验由 Android 设备验收，浏览器与本地协议测试不替代。

## 公共 HTTP/IP 与应用报告：本次适配顺序

1. **来源与网络**：接受共享接入材料中的 HTTP/HTTPS、IPv4/IPv6、ASCII/IDNA 域名及端口，拒绝下划线、尾点和非法 DNS label，展示规范化平台来源后沿用既有接入确认。Debug/Release 均允许所绑定 HTTP 平台的明文请求和 WebView 网络访问；不增加仅本机生效的开关或第二次 HTTP 特有确认。HTTPS 保持证书验证，不接受跨来源重定向。Android manifest 已有 `usesCleartextTraffic=true` 时无需再增加重复配置，但仍须检查各网络栈/WebView 的来源限制。
2. **地址使用**：使用资料中的 `platformUrl`，Discovery 返回同源 API path。Portal 使用 `portal/grants` 返回的同源 `exchangeUrl` 原生 POST 换票，不从二维码自造 Portal 地址。HTTP 接受非 Secure 的 HttpOnly Portal Cookie，HTTPS 使用 Secure；不让网页读取 ticket 或平台令牌。WebSocket 对 HTTP 使用 `ws`，HTTPS 使用 `wss`，不丢端口。
3. **同步触发**：进入企业空间、恢复前台、手动同步/Portal refresh 共用现有同步 owner；每次新顶层 Managed Runtime interaction 仍要 preflight。一次有效下载校验后整体提交配置及 generation/hash。网络失败保留完整已应用状态用于展示，按已有 guard 暂停新的企业运行请求；个人空间不受连带限制。
4. **应用报告**：将 `PlatformControlClient.reportApplied` 接入原子提交成功后的应用层流程，而非下载回调。请求体和错误见本文接入步骤 5 / OpenAPI。已成功应用但报告丢失时，下次检查重报相同值；401 经单一刷新 owner 处理，撤销停止报告。重登从新 Session 当前真实已应用状态重新报告，不借用旧回执。不要建立后台推送/通知子系统。
5. **设备验收顺序**：管理台发布 A → 手机扫码或粘贴 HTTP/IP 材料 → 同步并执行模型/语音/工具 → 管理台确认 A 已应用 → 管理台发布 B → 手机恢复前台或手动同步 → 确认 B 生效及报告 → 再测控制端断网、报告失败、428、退出/撤销和进程恢复。分别补 HTTPS 及 `ws`/`wss` 场景。设备报告不是当前在线证明，不是绕过 Runtime 准入的票据。

Android 的 `PlatformControlClient`、`PlatformSnapshotMapper` 和契约样例已有工作区实现时应核对并接入现有 owner，不重复创建平行客户端。导出包只含当前契约，不提供旧版本 DTO、迁移或协议回退。
