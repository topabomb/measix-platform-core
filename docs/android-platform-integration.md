# Android 平台协议消费说明

本说明面向 Android 维护方，描述当前源码的协议消费边界。Discovery protocolVersion="1"、Snapshot v4/v5、Portal Bridge v3 与 Enrollment formatVersion=1 是当前支持组合；新草稿发布 v5，保全已发布 v4。历史封版身份见 `docs/s0-execution-progress.md`，当前 Starter 实施及验证见 Core 源仓库的 `docs/starter-opening-snapshots.md`，两者不能混作同一部署证据。

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

### 企业与个人空间的关系

当前 v4/v5 平台执行不依赖后续 Gateway（预留 Snapshot v6）。Hub/Relay 没有以 S0.4 Freeze 为条件的 Runtime 开关；后续阶段的正式验收仍执行各自的门禁。

企业域复用 Android 现有聊天、流式输出、推理展示、工具循环、图片理解、图片生成、朗读、录音转写、助手、对话和本域记忆 owner。平台改变资源来源、认证、准入和归属，不应另造一个删减的聊天或媒体运行器。图片理解须同时满足模型资源声明 IMAGE、上游模型支持和原生编码；图片生成使用独立 `img_*` 资源和现有 `ImageGenerationCoordinator` / `GeneratedMediaStore`，不伪装为聊天 Model。平台不提供 Embedding 资源，也不提供云端对话/附件同步。个人原配置和数据不被企业配置覆盖。

| 可能阻止使用的条件 | 本轮明确的处理方式 |
| --- | --- |
| 原生仍拒绝 PLATFORM_ENROLLMENT | 实现并修复唯一 Platform source；不能靠 Portal 包或手机端模拟企业替代 |
| 手机不能访问开发电脑的 127.0.0.1 | 配置手机可达的 HTTP 或 HTTPS 统一公共入口（域名或 IP 均可），Hub PublicOrigin 保持相同；Discovery/Client/Runtime/Portal 都从该 origin 访问。具体 ingress 配置由 Core operations 文档维护 |
| 当前预览上游是合成服务 | 它只证明协议和转发；真实使用须在 Admin 配置实际供应商/CLIProxyAPI 地址、模型、凭据并应用、发布。客户端不接收企业密钥，不能把合成回复当成真实生成 |
| 五项 allowLocal* 为 false | 这是禁止企业域使用用户自带配置，不是禁用企业下发的资源。需要混用时由管理员启用对应策略并发布，不能由客户端越权绕过 |
| 辅助功能没有选中的可用资源 | 管理员可分别配置 fast/title/attachment-inspection/suggestion/compress 默认模型；每项均可留空，留空时沿用本域已有选择规则，不得把 defaultModelId 复制到其他槽位。图片生成使用独立的可选 `policy.defaultImageGenerationId`；未设置、资源禁用或显式引用失效时明确不可用，不静默切换到另一资源 |
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
| Model.modelId / upstreamModelKey | 前者是稳定平台资源 ID，映射 EnterpriseModel.id；后者虽沿用既有 wire 字段名，语义是 Core 发布给设备的有效模型标识（管理员别名，未配置则等于真实上游 key），映射请求模型名 modelId。真实上游 key 只在 Core 内部，不能与资源 ID 互换 |
| Model.displayName/modalities/capabilities | name 与显式 enum 映射。S0.2 为 CHAT；不映射成图片生成/附件等未声明 profile。未知 enum 拒绝候选，不能默认为普通文本模型 |
| Provider.clientProtocol + Model.runtimePath | 保存在平台运行适配配置中，决定公开 Relay URL 与请求 profile；不塞入本地私有 binding，也不向用户存储注入企业上游地址/密钥 |
| ImageGeneration.imageId/displayName/upstreamModelKey | 保持独立 `img_*` 企业资源身份；Android 只在统一图片选择目录中投影为 IMAGE 项，请求 body 的 `model` 使用 upstreamModelKey。不得并入 Provider/Model 表或借用 `mdl_*` |
| ImageGeneration.clientProtocol/runtimePath/maxImagesPerRequest/allowedSizes | 接受 `OPENAI_IMAGES_GENERATIONS` 与 `DASHSCOPE_MULTIMODAL_GENERATION` 两种同步 text-to-image typed profile。执行固定到该资源的 Relay 路径；Android 在排队与每次外部请求前复验数量和 canonical 尺寸，按协议构造 wire，禁止参考图、编辑、mask、multipart、partial、stream 与异步任务 |
| TTS.ttsId/displayName/clientProtocol | 保留企业资源身份，按 Control Protocol §10.5 显式分派四种语音执行方式。现有企业 OpenAI 专用通道需扩展，不能按模型名猜协议 |
| 云端 TTS.upstreamModelKey/voice/runtimePath/voiceDesignPrompt | 分别保留模型、预置音色、Relay 路径与 MiMo 描述。音色设计无 voice；Gemini JSON 内联 PCM，MiMo SSE 音频增量，OpenAI 二进制音频，各用对应编码/解码 |
| SYSTEM_TTS.speechRate/pitch | 仅设备执行，无上游绑定、Runtime URL 或企业服务器密钥。仍为企业资源，可在 allowLocalTts=false 时作为 defaultTtsId；缺少可用设备引擎须明确报错，不切换云端 |
| ASR.asrId/displayName/upstreamModelKey/language/clientProtocol | 保留资源身份、模型及可选语言；按 clientProtocol 分派 OpenAI multipart、DashScope HTTP JSON、OpenAI Realtime 或 DashScope Realtime。实时参数按当前协议映射，不把 WebSocket 转成文件上传 |
| MCP.mcpServerId/displayName/enabled/authOwnership | EnterpriseMcpResource.id/name/enabled；适配层保留 MCP_STREAMABLE_HTTP、runtimePath 和实际 authOwnership（ENTERPRISE_MANAGED 或 NONE）；均使用 Relay，无 OAuth/上游凭据下发 |
| Assistant.assistantDefinitionId/displayName/description/modelId/systemPrompt/mcpServerIds/enabled | EnterpriseAssistant 对应字段；缺省 description 可展示为空字符串；引用的 modelId 是平台稳定 ID，不是请求模型名 |
| Assistant.memorySeed[] | 按作者顺序转换为只读 Seed。内部 ID 从 deploymentId、助手 ID、generation、索引确定；空数组有效，条目不得为空白。不按内容去重、不建立可变 Assistant Memory 副本，配置替换时整体换代 |
| Starter.starterId/assistantDefinitionId/title/prompt/description/sortOrder/enabled | EnterpriseStarter 对应字段；仅启用且助手有效的入口可操作。展示按 sortOrder、starterId 排序。点击预填输入草稿，由用户发送，不新增 Portal 聊天写入 Bridge |
| Starter.openingSnapshot（v5 required；v4 不允许） | 消费 Core 编译后已固化的 System 与有序背景；Core Draft 的空白继承在编译时完成，Android 不从当前助手重新补空值。v5 选择时绑定 Draft，首发原子保存来源与背景；背景按字面处理，领域 System 沿 Android 既有 START 模板规则渲染。额外内容默认折叠、详情按需展开；v4 仍只预填。Core 编制与历史发布边界见 Core 源仓库 `docs/starter-opening-snapshots.md` |
| policy 五项 allowLocal* | EnterprisePolicy 五项必填 Boolean；缺失/null/错误类型均拒绝。只控制本企业域内用户原配置准入，不复制用户定义、端点和密钥 |
| defaultModelId/defaultFastModelId/defaultTitleModelId/defaultAttachmentInspectionModelId/defaultSuggestionModelId/defaultCompressModelId | 分别映射 defaults.chatModelId/fastModelId/titleModelId/attachmentInspectionModelId/suggestionModelId/compressModelId。六项都可省略且互不推导，非空引用必须指向已启用模型；附件检查默认还必须支持 IMAGE 输入 |
| defaultImageGenerationId/defaultTtsId/defaultAsrId/defaultAssistantId | 分别映射 defaults.imageGenerationModelId/ttsId/asrId/assistantId；显式无效引用不回退首项。没有用户已选助手时采用 defaultAssistantId；用户已选助手失效时呈现选择与修复入口，不静默改选默认助手。未提供的默认值保持未指定，由既有本域选择规则处理 |
| allowAsSubAssistant/allowedSubAssistantIds、gateways | v4/v5 不下发企业子助手关系或 Gateway；平台适配输出 false/空集合。用户自有子助手仍按现有五项准入和执行权限处理，不扩展 wire |

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
| GOOGLE_GENERATE_CONTENT | runtimePath 使用 Core 发布的有效模型标识，例如 `/v1beta/models/{upstreamModelKey}:streamGenerateContent`，追加 `?alt=sse`；Core 再映射为真实上游路径 | contents/parts、functionCall/functionResponse；保留模型返回的调用 ID 和 thoughtSignature；解析 candidates |
| ANTHROPIC_MESSAGES | runtimePath 通常为 `/v1/messages` | messages、独立 system、max_tokens、stream=true；tool_use/tool_result 配对；anthropic-version 公开协议头；解析命名 SSE 直至 message_stop |
| OPENAI_IMAGES_GENERATIONS | `/runtime/v1/resources/img_ffffffff-ffff-4fff-8fff-ffffffffffff/v1/images/generations` | 同步 JSON `{"model":"gpt-image-1","prompt":"...","n":1,"size":"1024x1024"}`；响应只接受 `b64_json` 或安全 HTTPS URL，下载不携带平台 Bearer/Cookie，并在交给 `GeneratedMediaStore` 前执行有界读取与图片签名校验 |
| DASHSCOPE_MULTIMODAL_GENERATION | `/runtime/v1/resources/img_ffffffff-ffff-4fff-8fff-ffffffffffff/api/v1/services/aigc/multimodal-generation/generation` | 同步 JSON `model/input.messages/parameters`；Android 将 canonical `1024x1024` 只在 wire builder 转为 `1024*1024`，显式发送 `n` 与 `watermark=false`，从 `output.choices[].message.content[].image` 读取安全 HTTPS URL |
| OPENAI_AUDIO_SPEECH | `/runtime/v1/resources/tts_cccccccc-cccc-4ccc-8ccc-cccccccccccc/v1/audio/speech` | JSON `{"model":"tts-1","voice":"alloy","input":"你好"}`；读取二进制音频，不当 JSON/SSE 解析 |
| GEMINI_GENERATE_CONTENT_TTS | runtimePath 中的 `:generateContent` 路径 | responseModalities=AUDIO，speechConfig 设置下发 voice；解码 inlineData 中的 PCM 和采样率 |
| MIMO_CHAT_COMPLETIONS_TTS | runtimePath 通常为 `/v1/chat/completions` | 按当前 Android MiMo 编码器构造 audio/messages，分别消费标准 voice 或 voiceDesignPrompt；读取 SSE 音频增量 |
| SYSTEM_TTS | 无 Runtime URL | 设备系统引擎执行 speechRate/pitch；保持企业资源归属，allowLocalTts=false 不禁用这个企业定义 |
| OPENAI_AUDIO_TRANSCRIPTIONS | `/runtime/v1/resources/asr_dddddddd-dddd-4ddd-8ddd-dddddddddddd/v1/audio/transcriptions` | multipart/form-data：model=whisper-1、language=en、file=<录音>；由 HTTP 库生成 boundary。解析 transcription 响应；不是 WebSocket ASR |
| DASHSCOPE_HTTP_ASR | 资源自己的 `/api/v1/services/aigc/multimodal-generation/generation` runtimePath | POST application/json：`model=upstreamModelKey`，`input.messages` 最后一条用户消息带 `input_audio.data=data:audio/wav;base64,...`，`parameters.format=wav`（MP3 同理）；从 `output.text` 读取文字。可选语言放 `parameters.language_hints=[language]`。不发送 multipart、实时参数或 WebSocket 握手 |
| OPENAI_REALTIME_TRANSCRIPTION / DASHSCOPE_REALTIME_ASR | 同一资源路径规则，HTTP 平台使用 `ws`、HTTPS 平台使用 `wss`，保留端口；分别追加 `intent=transcription` / 编码后的 `model` query | GET WebSocket 握手保留三项平台头。按下发 sampleRate/vadThreshold/silenceDuration 及协议专属 prefixPadding/prompt 建立会话；取消或退出关闭连接。完整事件与字段见 Control Protocol §10.6，不能改成文件上传 |
| MCP_STREAMABLE_HTTP | `/runtime/v1/resources/mcp_eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee/mcp/v1` | 标准 MCP Streamable HTTP 会话：initialize → initialized → tools/list → tools/call；Accept 同时允许 application/json 和 text/event-stream。沿用当前 MCP 客户端维护返回的会话/协议 header，不自创固定服务端 Session，不经 Gateway |

Direct MCP 的平台路由允许 `POST`、`GET`、`DELETE` 到 Snapshot 给出的同一 `runtimePath`；Android 保留上游会话 header。GET 事件流是服务端可选能力，上游返回 405 应按“不提供该流”处理；不应收到 Core 的 `ROUTE_POLICY_DENIED` 403。会话终止时的 DELETE 结果依上游会话状态处理，不把无 Session ID 时的 400 误判为平台拦截。

完整 428 body 使用 [现行 Problem 样例](../api/fixtures/problem/managed-snapshot-required.json)。这些是请求构造示例，不声称合成 profile 已取得真实供应商资格认证。

## 远程工作区与文件客户端

共享配置仍为 Snapshot v5；远程工作区状态不塞入企业共享 Snapshot，也不能根据托管 MCP 的名称推断。使用当前企业身份的 Bearer 调用 `GET /api/client/v1/workspace`，消费独立 `WorkspaceProjection`（schemaVersion 1）。本协议尚未生产发布，Core 和消费者直接同步现行合同，不增加旧工作区协议探测、字段缺省或回退分支。

| 字段 | 客户端含义 |
|---|---|
| `serviceState` | `NOT_CONFIGURED` 企业尚未配置；`DISABLED` 企业已关闭；`ENABLED` 企业配置为启用，不表示远端健康或用户可访问 |
| `state` | 当前用户空间生命周期；企业启用且 `UNPROVISIONED` 表示尚未给当前用户开通 |
| `filesAvailable` / `filesReason` | 是否允许进入文件操作及不可用原因；不依赖 MCP 发布、预算或工具可用性 |
| `mcpAvailable` / `mcpReason` | 助手工具独立状态；不能作为文件入口开关 |
| `agentSpaceId` | 本次操作的固定空间身份；不是由客户端指定任意用户的授权参数 |

登录、企业切换、前台恢复、进入文件页面以及访问失效时重新查询；请求失败应显示未知/重试读取，不能伪装为企业关闭。未启用时隐藏文件操作，已启用未开通时说明需管理员开通；已有空间不可用时保留原因和刷新入口。状态查询不启动 VM。文件读取和工具执行可能启动 VM，应允许取消并呈现加载状态。

文件 API 基址为 `/api/client/v1/workspace`。所有 `files`/`content` 请求必须带当前投影取得的 `agentSpaceId` query，路径为相对工作区根目录的路径，URL 编码一次。Client 用户完全来自 Bearer。列表、预览、下载缓存与草稿按企业身份、空间 ID、路径及文件 ETag 隔离；重新开通的新空间不能继承旧操作目标。空间身份或登录身份变化时取消旧请求，重新查询后由用户发起新操作。

| 操作 | 合同与客户端要求 |
|---|---|
| 浏览与容量 | `GET files?agentSpaceId=…&path=…`，返回条目、文件版本、可用时的容量；不支持任意分页伪装，目录超限须明确报告 |
| 新建目录、重命名、移动、复制、删除 | `POST files?agentSpaceId=…`，使用 `WorkspaceFileMutation`；源 ETag、目标覆盖条件、递归确认按 OpenAPI；禁止根目录修改 |
| 下载与预览 | `GET/HEAD content?agentSpaceId=…&path=…`；正文流式落盘，保留 ETag、长度、类型等元信息。预览按实际类型和大小限制，不执行 HTML/SVG，不自动访问文档外部资源 |
| 续传与缓存 | 续传使用 Range 加同一强 ETag 的 If-Match，版本变化返回 409；条件缓存读取使用 If-None-Match。206 核对 Content-Range 后续传，200 必须重建完整文件，不能追加；304 仅使用同身份、同空间、同版本的完整缓存；416 重新核对长度与版本 |
| 上传或新建文本 | `PUT content?agentSpaceId=…&path=…`，原始字节正文；新文件必须 `If-None-Match: *`，覆盖必须单个强 `If-Match`，两者不能并用；流式上传、进度、取消由客户端负责 |
| 编辑文本 | 从同一次 GET 取得正文和 ETag；UTF-8 严格解码，保留 BOM 与换行格式。Admin 首轮限制 2 MiB，非 UTF-8/二进制/混合换行只预览或下载。保存携带该 GET 的 ETag；没有强 ETag 时只允许另存新文件。未保存退出需确认 |
| 分享 | 下载到应用私有临时文件后由 Android 原生文件分享授权给用户选择的应用；分享的是文件副本，不是 WebDAV 凭据、Core Bearer 或公开 URL。Core 不新增分享链接/分享会话接口 |

HTTP 成功不代表所有写操作成功，必须检查 `WorkspaceFileResult.outcome`（`SUCCEEDED` / `PARTIAL` / `UNKNOWN`）和失败条目。错误 body 使用现有 Problem 的 `code`，不能靠中文消息分派：

| HTTP / code | 处理 |
|---|---|
| 400 | 无效路径、空间参数或条件；修正请求，不重试原写入 |
| 401 / 403 | Core 身份失效/权限不足，走原认证或权限流程；DAV 失效不会映射为 Core 登出 |
| 404 `file_not_found` | 文件已不存在，刷新目录 |
| 409 `workspace_space_mismatch` | 固定空间不匹配，刷新工作区，保留旧目标信息，禁止把旧写入转发到新空间 |
| 409 `file_version_conflict` / `file_conflict` / `file_locked` | 保留编辑，重新读取核实或另存新文件；不得自动强制覆盖 |
| 416 `file_range_invalid` | 核对远端版本和长度后重新下载 |
| 422 `file_listing_limit` / 429 `file_transfer_limit` | 目录超限/并发已满；提示缩小目录或等待已有传输结束 |
| 503 | 工作区、DAV 凭据、传输失败或结果未知；刷新状态并核实目标，不自动重放写入 |
| 507 `file_storage_full` | 空间不足，保留本地内容，释放空间后由用户决定后续动作 |

取消、网络断开或未知结果都不能证明写入未发生。重新读取可安全重试；覆盖、创建和删除不得自动重放。读取失败不能丢失尚未保存的编辑，成功保存后再关闭或以一次新 GET 开始下一轮编辑。Android 的后台传输、文件分享授权和本地缓存清理属于客户端 owner，本轮 Core/Admin 验证不等于这些设备能力已交付。

## 合同回归入口

`api/fixtures/client-integration/` 提供基础、四模型协议、两种文生图、四种 TTS、四种 ASR、策略拒绝、HTTP 时序及 Runtime 请求样例。接入资料、Portal Native Bridge 与 Feed 的正反例分别在 `api/fixtures/enrollment/` 和 `api/fixtures/portal/`。Android 应消费同一份 Core 导出并对当前资料进行严格解析、引用校验及执行分派；导出包本身不包含真机运行结果。

Core 的合同、身份/会话与 Relay 测试证明服务端行为，Portal 生产浏览器测试证明网页及 Hub 生命周期；Android 的 Realm、WebView、相机/麦克风、系统朗读与资源运行仍以对应设备记录为准。封版记录只引用实际完成的场景结果，不把某次历史模拟器失败或浏览器替身当作当前设备状态。
