# S0.2 生产用量解析与用户额度实施方案

> 状态：已实施；本文保留为 S0.2 用量与额度实现、验证和维护边界。
> 产品/阶段权威：[S0.2 用量与额度收尾合同](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-usage-budget-closure-contract-spec.md)。
> Wire authority：[Control Protocol](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md)；精确 HTTP schema 在本仓库 OpenAPI 中实现。
> 本文拥有具体代码组织、数据与事务方案、实施顺序和检查要求，不另定义预算产品语义。

## 1. 实施后的当前基础

- `backend/internal/relay/runtime` 负责透明代理、统一预算准入和请求观察；`protocolusage` 负责生产协议解析，`relay/metering` 以同一 SQLite spool 承载批量投递、重试和恢复。
- `hub/usage` 负责语义计量、归属、查询与结算；`hub/budget` 负责规则、自然周期计数、原子占用、幂等结算和待核对状态，不以查询聚合代替准入计数器。
- Admin 的 Usage 与 Users 页面提供筛选、趋势、请求详情、五类能力的用户额度、限额模板、能力覆盖、审计与待核对处置；正式删除用户使用精确用户名、原因、幂等状态机和不可逆凭据 tombstone。
- Portal 在受限本人 Session 下提供首页摘要及“我的用量与额度”，复用 Core 投影，不持有预算真源。
- Android 消费当前 Client OpenAPI，在企业空间展示本人额度，并在 Model、Image Generation、TTS、ASR、Realtime ASR 与 MCP 的平台运行时边界统一解析结构化 Problem；固定 Portal 深链不开放任意 URL。

只支持当前合同。本文描述已落地的 owner、事务和验证边界；本轮运行证据继续维护在 `docs/s0-execution-progress.md`。

各端从未发布，本任务不编写旧库迁移/数据转换/兼容分支；直接更新 Ent/schema 与唯一当前初始化 SQL（现有目录即使名为 migrations，也只维护当前初始化用途）。实际实施时，用户已授权直接删除重建本任务相关本地测试数据库及持久化，包括已确认归属的 Hub 测试库、Relay 测试 spool、预算测试数据和专用测试环境企业缓存。先停止使用这些文件的测试进程、解析并核对绝对路径，连同 SQLite 辅助文件一起处理，再用当前初始化和 fixture 重建；记录删除对象及重建结果，不重复申请同范围批准。不删除 Android 个人对话/设置、Secret/密钥、其他项目持久化或未指定远程数据。测试设备若企业缓存与个人数据不能安全区分，使用独立测试 profile/实例而不是清空用户整机数据。

清理是 schema 开发期重建手段；故障恢复和幂等验证必须保留测试中的在途/spool 数据模拟重启，不能靠清库伪装恢复成功。本文修订本身不执行删除。

## 2. 代码组织与依赖

```text
backend/internal/relay/
  runtime/         现有代理生命周期，增加计量观察与预算准入接点
  protocolusage/   统一请求级结果、协议注册、JSON/SSE/语音计量
  budget/          受服务认证的 Hub 准入客户端
  metering/        扩展现有 spool/sender，不增加第二套 outbox
backend/internal/hub/
  usage/          统一入账、归属验证、查询与聚合
  budget/         用户规则、周期计数、原子占用、幂等结算/核对
  httpapi/        Admin 和 Portal 的权限投影
console/src/
  pages/          扩展 UsagePage 和 UsersPage
  components/     复用请求列表；增加需要复用的额度呈现组件
```

按协议文件组织 production Adapter，共享基础流读取；不要为每个供应商品牌建立包、服务、配置 DSL 或插件加载系统。一个请求一个解析状态。解析器不依赖 Ent、预算服务或 UI；Relay 不 import Hub domain/Ent，不访问 hub.db。仅共享直接生成的 wire 和无业务权威的基础工具。

Portal 代码在同级仓库维护；增加个人用量/额度界面及 source/session 查询入口，复用认证失效、请求取消和迟到响应隔离，不另建后端或复制 Admin 页面状态。

## 3. 生产协议 Adapter

协议由 Hub 从已发布资源 clientProtocol 编译入 Relay route；模型/TTS 相同路径依靠资源类型及协议区分。编译规则、允许枚举、控制 hash 和校验测试同步修改。

统一生命周期包含：创建观察器、提取可预知请求量、增量观察响应、终结结果。最终结果是结构化 meter/source/completeness 和诊断，不含正文。HTTP JSON、SSE、binary、multipart 和 WebSocket 使用同一结果模型，但不强迫使用同一种读取器。

必须在 Core 这一轮整体实现下列 14 个云端协议/profile（4 LLM + 2 Image Generation + 3 TTS + 4 ASR + 1 MCP），不能只实现一个协议后转去其他仓库。SYSTEM_TTS 明确不做服务端计量。最小实现由下表限定；协议解析支持与某个供应商模型的真实资格分别记录，不能用 LEVEL_0 掩盖缺少本表规定的生产解析代码。

### 3.1 LLM：字段、合并与最小指标

四种 LLM 均必须产出一次调用、INPUT_TOKENS、OUTPUT_TOKENS、TOTAL_TOKENS；单个请求不能取得某项时该项显式 UNKNOWN/PARTIAL。缓存/推理为可得明细，不额外扣第二份预算。整数非负、字段关系矛盾、相同事件重复及 stream 中断均需有固定样例。

| clientProtocol | 最小传输与取值 | 合并/归一化方法 |
|---|---|---|
| `OPENAI_CHAT_COMPLETIONS` | POST chat/completions 的 JSON 与 SSE；`usage.prompt_tokens`、`completion_tokens`、`total_tokens`；`prompt_tokens_details.cached_tokens`、`completion_tokens_details.reasoning_tokens` 为明细 | 非流式读响应 usage；流式仅合并非 null usage，最终 usage chunk 可在 finish_reason 之后、choices 可为空。保存最后一个完整请求级 usage，不能累加所有 chunk。输入=prompt，输出=completion；总量优先 total，缺总量但输入输出完整时相加；缓存/推理已包含在对应大项。缺最终 usage 的 DONE 不代表零 token。 |
| `OPENAI_RESPONSES` | POST responses 的 JSON 和 HTTP SSE（当前 Android 使用 stateless `store=false`）；JSON `usage` 或终态事件 `response.usage` 的 `input_tokens`、`output_tokens`、`total_tokens` | 观察 `response.completed`，也处理 `response.incomplete`/`response.failed` 中实际存在的 usage；不能从 output_text.delta 字符估 token。依据 response id 和事件序号去重，终态快照覆盖进度，输入/输出明细分别从 `input_tokens_details.cached_tokens`、`output_tokens_details.reasoning_tokens` 读取。业务 incomplete 不自动等于计量不完整，有完整可靠终态 usage 可 EXACT。 |
| `GOOGLE_GENERATE_CONTENT` | POST generateContent 的 JSON 和 streamGenerateContent?alt=sse；`usageMetadata.promptTokenCount`、`candidatesTokenCount`、`thoughtsTokenCount`、`totalTokenCount`、`cachedContentTokenCount` | 请求级 metadata 按出现字段更新，不逐 chunk 相加；INPUT=promptTokenCount（含缓存），OUTPUT=candidatesTokenCount + 明确可得 thoughtsTokenCount；TOTAL 优先供应商 totalTokenCount。缺 thoughts 不擅自推定为零，仅在已验证不推理 profile 可按约定零处理。总量与可解释细分不吻合时保留供应商总量并标记明细差异，不能把差额硬塞入输出。toolUsePromptTokenCount 等保留诊断明细，不盲加进预算。 |
| `ANTHROPIC_MESSAGES` | POST messages 的 JSON 与命名 SSE；JSON `usage`，流式 `message_start.message.usage` 和 `message_delta.usage` | 按字段合并最后已报告累计值，缺字段保留原值；message_delta usage 不是增量，不能重复相加。INPUT=`input_tokens + cache_creation_input_tokens + cache_read_input_tokens`；OUTPUT=`output_tokens`；TOTAL=INPUT+OUTPUT。cache_read 为缓存命中明细，cache_creation 为独立输入明细；只有官方/已验证 profile 明确可缺省为零的可选缓存字段才补零。message_stop 是终态，缺必需 usage 仍不完整。 |

Chat Completions 的企业 Android 请求在 stream=true 时必须发送 `stream_options.include_usage=true`，已有 builder 的 vendor 例外要按企业受信协议能力核对；Relay 不偷偷补写请求 body。不接受 include_usage 或不返回 usage 的兼容供应商保留透明调用能力，但不得标记为可执行 token 预算。Core 阶段使用正确生产形状的协议客户端验证，Android 阶段同步实际 builder。Responses 不增加服务端会话或 previous_response_id 依赖，原生工具往返仍遵守当前合同。

### 3.2 Image Generation：同步 text-to-image

`OPENAI_IMAGES_GENERATIONS` 只接受已发布 `img_*` route 的同步 `POST /images/generations`。Relay 从受信 JSON 请求有界读取 `n` 与 `size`：`n` 缺失规范化为 1，必须位于 1..资源 `maxImagesPerRequest`（平台上限 6），`size` 必须属于资源 `allowedSizes`。`DASHSCOPE_MULTIMODAL_GENERATION` 只接受精确原生 generation path 与固定 `model/input.messages/parameters` 结构，要求显式整数 `parameters.n`、`parameters.size=宽*高` 和 `watermark=false`；Relay 仅把 `*` 映射为 canonical `x` 做 allowlist 比较。两种合法请求都保持 body 字节与供应商响应透明，不解析 prompt，不下载或持久化图片，不在协议间转换结果。

每次实际开始的上游调用计 `REQUESTS=1`，`REQUESTED_IMAGES=n`；它表示请求图片数，不冒充返回或保存成功数。明确未转发才释放两者。有限图片预算无法可靠解析合法 `n` 时 fail closed；无限模式也执行资源单次上限与 size 准入。edit、图片输入、reference、mask、multipart、stream、async 不在当前 profile 内，不能由通用代理路径绕开。

### 3.3 TTS：最小按输入字符结算

统一 CHARACTERS 口径为目标文本字段经 JSON 解码后的 Unicode 码点数，包含空白、换行、标点和字段内风格标签；不 trim、不按 UTF-8 字节或 UTF-16 单元计数。平台量表示“提交的朗读文本字符”，不承诺等于供应商计费或实际发声字符。合法目标文本完整读取后才可准确预占；在明确有界 JSON 内观察并原样重放，避免先转发后检查字符预算。

| clientProtocol | 必交的生产读取方法 | 本阶段不要求的量 |
|---|---|---|
| `OPENAI_AUDIO_SPEECH` | 从请求 `input` 字符串计 CHARACTERS，`instructions` 和 voice 不计；响应 binary 或协议允许的音频事件仍原样转发；一次已发起调用计 REQUESTS=1 | 不靠 MP3/Opus 输出字节估播放时长，不要求供应商 token；已可靠取得时可作为额外明细。 |
| `GEMINI_GENERATE_CONTENT_TTS` | 对当前单个用户输入的 `contents[].parts[].text` 计数，多个 text part 分别计数后相加且不人为插入分隔符；内嵌朗读指令包含在该平台文本口径内，speechConfig 不计。JSON inlineData 音频不为字符预算解码 | 不区分自然语言提示中的“指令字符/正文字符”，不通过模型预测实际朗读内容，不要求音频 token 预算。 |
| `MIMO_CHAT_COMPLETIONS_TTS` | 对当前 profile 最后目标 `role=assistant` 消息的字符串 content 计数；前置 user 声音设计/风格描述不计；JSON WAV 和 SSE PCM16 两种响应均透传 | 不将 LLM Chat Completions 的 token 必需项套到 MiMo TTS；无 assistant 文本的自动生成/润色特殊模式不声明支持精确字符预算。 |

目标字段格式超出上述最小 profile 时，无限或仅次数预算仍可按既有传输规则执行并标记字符 UNKNOWN；启用了字符预算则在转发前返回 `usage_meter_unavailable`，不能绕过字符预算。首次支持的三种云 TTS 正常文本调用必须均为 CHARACTERS EXACT；不能以该降级规则替代实现。

### 3.4 HTTP ASR：最小音频格式与时长

必交输入是当前 Android 录音使用的 RIFF/WAVE PCM16 little-endian（合法声道/采样率由资源与上游约束）；最小验收包含单声道 16 kHz 和 24 kHz（只对供应商支持者执行真实调用）。用 RIFF chunk scanner 读取 fmt/data，跳过未知 chunk 与 padding，按实际完整样本数、声道和采样率计算。检查 blockAlign、截断和声明 data 长度，不能仅信 header 填值或所有 WAV 都固定 44 字节。

| clientProtocol | 生产提取方法 | 预算及其他格式行为 |
|---|---|---|
| `OPENAI_AUDIO_TRANSCRIPTIONS` | 增量观察 multipart 的 `file` 音频 part，不把 multipart 边界/其他字段计入时长；WAV 按实际提交样本统计。响应 JSON 的 `usage.type=duration`/`usage.seconds` 或 verbose JSON `duration` 可作为有来源的供应商时长；`usage.type=tokens` 只存可得 token 明细，不能换算秒数 | WAV 的平台 AUDIO_SECONDS 优先实际样本口径；非 WAV 可使用已验证的供应商 duration（事后结算）。首次完整交付必须支持 WAV；MP3/M4A/WebM 等无可靠时长来源时只声明请求计数，启用时长预算的未认证格式在上传前尽早拒绝，不引入 ffmpeg 或完整媒体解码框架。 |
| `DASHSCOPE_HTTP_ASR` | 增量读取 `input.messages[].content[].input_audio.data` 的 Data URI，增量 Base64 解码后送同一 WAV scanner；`parameters.format/sample_rate` 只作一致性检查。当前非流式响应解析 `usage.duration`（秒），不把 output.text 字数/句子 end_time 当音频总时长 | WAV 按样本计平台时长；供应商 duration 独立记录并显示来源/精度，不能重复加入 AUDIO_SECONDS。URL 音频不由 Relay 另行 fetch；只在已验证供应商完整 duration 可用时支持其事后时长预算。新官网的 HTTP ASR SSE 句级 usage 不在当前最小 Android profile 内，不猜其增量/累计关系；启用时长预算且不受支持时明确拒绝。 |

采用“实际提交音频时长”作为 WAV/实时 ASR 共同平台额度口径；供应商处理/舍入时长是另一个有来源的诊断值，不覆盖已有精确样本计数。样本不完整保留已确认的量并标 PARTIAL。压缩格式供应商来源若作为预算量，需在资源能力及 Admin/Portal 明细中标明来源，禁止静默换单位。

上传字节只做有界观察，不整体加载内存。时长准入统一采用已结算达到额度后停后续；本阶段不为 HTTP 文件先完整上传到 Relay 再预占整文件时长。取消后按已确认实际提交样本结算，不能按原文件完整长度扣整段。秒到毫秒按一次请求终结时作固定向上取整；PCM 分块保留样本分子/余数，不能每块取整导致累计偏差。

### 3.5 实时 ASR 与 MCP

| clientProtocol | 必交的生产解析 | 去重与结束规则 |
|---|---|---|
| `OPENAI_REALTIME_TRANSCRIPTION` | GET 升级前做预算准入；当前 `session.update` transcription profile 的 `audio.input.format` 为 `audio/pcm`、24 kHz、PCM16 单声道；仅对已提交上游的 `input_audio_buffer.append.audio` Base64 音频累计样本时长 | 使用服务端 session.updated 确认的有效格式；成功配置前的未知音频不能按默认采样率猜。一次连接一个 REQUESTS；转写 delta/completed 不产生第二次请求量。clear 不退回已提交音频；应用层再次 append 是新提交，TCP 重传不重复计量。 |
| `DASHSCOPE_REALTIME_ASR` | 当前 `session.update` 的 `input_audio_format=pcm`、`sample_rate=8000/16000` PCM16 单声道；对 `input_audio_buffer.append.audio` 解码计样本；响应文本不是时长来源 | 根据已确认配置分段计时；忽略非音频事件，session.finish/finished 以及关闭只触发一次最终结算。被拒绝的格式修改不改变当前计量参数。正常关闭与异常断线都保留已提交量。 |
| `MCP_STREAMABLE_HTTP` | 使用现有 Relay 请求事实，不新增 JSON-RPC 用量解析；每次开始上游 HTTP 请求扣一次（包含 POST 通知、GET 流及 DELETE），平台拒绝不扣 | 重连为新请求，SSE 内多个事件不增加请求数；工具级消耗不是本项指标。无限和按次数额度均适用，长连接仍受现有运行限制。 |

WebSocket 生产观察器必须支持文本帧分片、continuation、控制帧穿插、mask 与协商压缩；对原始帧只读旁路解码，保留原帧转发。无法处理的协商不能静默标为可计量；有限音频预算要求受支持的协商组合。计量失败后仍将已知量写入 spool 并进入待核对，不能把未解决记录自动清零。

### 3.6 共同完成标准和官方依据

每个 profile 都交付生产实现、声明的 meter capability、正常/缺字段/分块/取消固定输入与预期结果，适用时还有缓存、推理和格式变化用例。capability 必须是“协议 + 当前配置/模型/格式可证明的指标”，不能仅根据枚举宣告所有兼容供应商都精确。TOTAL_TOKENS 达量停后续；无法取得必需指标的资源在启用对应预算时失败关闭，次数或无限模式继续依照正常传输合同。Core 的集中验证一次覆盖全部 profile，后续仓库不重新实现计量。

最低固定算例：Chat/Responses 输入 100、输出 20、缓存 40、推理 5 → 总量 120（不变成 165）；Gemini prompt 100、candidate 20、thought 5、total 125 → 输出 25/总量 125；Claude input 10、cache creation 30、cache read 60、output 20 → 输入 100/总量 120。重复最终 usage 不增加这些数值。TTS 文本 `你😀 A` 为 4 个 Unicode 码点。16 kHz 单声道 PCM16 的 32,000 有效音频字节为 1 秒，分任意块结果相同；RIFF/multipart/Base64/WS 封装不增加时长。上述为合成验证算例，不冒充供应商实测。

本节字段依据于本次核对的官方页面；仅固定当前已声明 profile，不随官网新增产品自动扩大范围。以下链接供实现时核对差异和保存脱敏 fixture，不要求逐协议另立实施任务：

- [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)：usage、include_usage 和流中断；[Responses usage](https://developers.openai.com/api/reference/cli/resources/responses/methods/retrieve)：输入/输出/总量；事件解析同时使用当前仓库 Responses 样例。
- [Gemini GenerateContent](https://ai.google.dev/api/generate-content)：usageMetadata；[Gemini TTS](https://ai.google.dev/gemini-api/docs/speech-generation)：contents/text 音频生成。
- [Anthropic Streaming](https://platform.claude.com/docs/en/build-with-claude/streaming) 与 [Prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)：累计事件、三部分输入 token。
- [OpenAI Speech](https://developers.openai.com/api/reference/resources/audio/subresources/speech/methods/create)、[Transcriptions](https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create)、[Realtime transcription](https://developers.openai.com/api/docs/guides/realtime-transcription)。
- [MiMo TTS](https://mimo.mi.com/docs/en-US/quick-start/usage-guide/audio/speech-synthesis-v2.5)：assistant 目标文本和 user 风格消息。
- [DashScope HTTP ASR](https://help.aliyun.com/zh/model-studio/fun-asr-flash-recorded-speech-recognition-http-api)、[Realtime client events](https://help.aliyun.com/zh/model-studio/qwen-asr-realtime-client-events)。

共同技术要求：

- SSE 处理任意网络分块、CRLF、多行 data、空事件、终止与 usage 分离；不得假定一 Read 对应一事件。
- 流式累计/增量语义按协议处理，不把多次累计 usage 相加。非流式和流式均覆盖协议支持的形式；缺少最终 usage 标记不完整。
- 总 token 增加标准 TOTAL_TOKENS；保留输入、输出和可靠细分的语义关系。缓存/推理明细不再次加入总量，也不作为费用行重复计算。费用不是本项工作。
- 请求观察有界；TTS 只统计实际朗读字段；大型 ASR 上传流式处理，不能为预算整体加载音频。无法预知的时长走实际结算，不推断精确预占。
- WebSocket 观察必须处理帧分片、消息重组和已协商编码，受读上界约束。PCM 时长按有效样本、采样率和声道计算；不能拿 TCP 字节或会话时长代替。
- 保持原始传输顺序、逐步 flush、取消与背压；观察器失败走结构化诊断，不篡改上游内容。语义严格预算不可用时禁止将该资源显示为已支持。
- HTTP `Content-Encoding` 只在 Relay 的有界私有观察副本内解码后解析；客户端仍收到原始压缩字节和原始响应头。解码失败或超过观察上限时记录结构化未知用量，不能修改、截断或二次编码代理响应。
- 不保留完整消息、文件或音频；fixture 使用合成/脱敏数据，不携带 Secret。

## 4. 用量事件与账本

扩展现有 `api/internal/usage-ingest.openapi.yaml` 的交付能力，保留请求事实与语义结果的明确类型。可在同一 durable envelope 中交付，不另建品牌专用 endpoint。requestId 是唯一平台请求关联，meter 事件具有稳定身份/版本；生产服务重试不得重新生成去重身份。

实现不能仅逐条调用当前 RecordSemantic：应增加事务内入账入口，同时完成归属核验、幂等处理、语义落库及预算结算。校验用户、资源、上游、协议与准入捕获的配置一致；来源不得自报任意 userId。语义先于请求事实到达可暂存待关联，未验证关联不得混入他人查询或扣错预算。

计量记录需要来源、完整度、协议归属及 settlement revision。同 requestId 同 revision 同内容幂等；相同 revision 不同内容拒绝并诊断；后续可靠修正只应用差额且留历史。避免 sourceEventId 每 meter 重复导致其他指标被丢弃。保留事实事件与修正轨迹，不用修改资源当前名称解释历史。

清单查询保持时间窗和 keyset 分页；用户/协议/资源汇总和按日趋势采用数据库聚合，避免加载全量 rows 或每条请求 N+1 查询。额度读写走独立计数桶，不能每次扫描使用历史。明细保留策略不能清空累计预算；统计归档与预算权威分离。

## 5. 预算存储与事务

使用现有 Hub SQLite/Ent 增加三类逻辑数据：

1. 用户预算配置/规则：用户、现有能力类别、UNLIMITED/LIMITED、配置来源、周期、指标上限、生效区间、配置 revision；配置审计包含操作者与前后值。无限不使用数值哨兵；没有配置由服务端明确投影 DEFAULT 的无限状态。
2. 周期计数桶：稳定规则作用域、周期起点、指标、已结算量与可确定的预占量。累计桶不因编辑、停用/重启或详情清理而重建。
3. 请求占用/结算记录：requestId、可信身份/资源/协议、准入时间、匹配规则与周期、预占值、状态、最后结算 revision。

整数指标使用足够范围的整数，音频以整数毫秒存储和运算，在 UI 转为分钟；禁止浮点累计额度。TOTAL_TOKENS 与其明细不重复更新总量计数。

Hub 原子准入事务检查所有适用规则、周期和用户在途数，再创建唯一 requestId 占用并返回决策。使用数据库条件更新/事务串行化处理竞争及 SQLite busy，不能用进程 map 锁代替数据库原子性；网络调用不置于 DB 写事务内。起步让每次企业 Runtime 调用经过这个轻量准入入口，避免先建设规则缓存失效协议；因此 Hub 可用性成为 Runtime 准入依赖，必须在 operations 和 readiness 中说明。

可预知字符/时长同时预占；未知最终 token 只检查已知耗尽状态并占一个在途位置，不能伪造精确 token 预留。请求数、字符硬上限和 token 到量停后续的差别由 Hub 状态明确投影给两端。

Relay 在转发之前持久记录准入/执行身份，并将允许、开始尝试和最终结果纳入可恢复状态。若 Hub 已占用但回复丢失，以相同 requestId 重试获得同一决策；超时不换 ID 二次占用。只能证明未开始转发时释放；转发发生与进程崩溃之间无法判定的窗口保持待核对。已有 spool 需要扩展为可承载这些生命周期记录，而非只在响应结束后首次写入。

结算事务一次性去重、入账、更新周期计数、释放已解决的在途占用。Relay 收到 durable ACK 后才清理已交付记录。重启从 durable 状态恢复，不能按短 TTL 无条件退回名额。待核对记录提供 Admin 查询与受审计的人工处理入口；迟到可靠计量仍能修正，不能重复扣整笔或冲掉审计。

额度配置独立于 Managed Release，采用 expectedRevision 防覆盖，后续准入读取最新规则；已准入请求保留捕获的周期归属。有限切换无限仍保留并累计既有规则窗口的实际消耗，恢复有限不重置计数；全新规则从明确启用时刻计算。无限请求仍需可靠用量记录和有限运行保护，不建立虚构额度占用。自然周期由部署 IANA 时区计算，时区修改不得悄悄重置活动桶；第一版将额度时区固定为部署约定并在 UI 显示。

## 6. 接口与下发合同变更清单

保持 API v1、Snapshot v4、Bridge v3 以及现有版本号不变。修改精确 schema 后执行 generation，更新共享 fixtures/哈希和各消费者，不支持旧原型。业务 revision 正常递增。下表名称为拟定 endpoint，落地以 OpenAPI 为唯一精确权威。

| 文件/合同 | 实施要求 |
|---|---|
| `api/internal/relay-control.openapi.yaml` | route 增加可信计量协议/必要静态参数，更新编译、控制状态校验与 hash；不下发每用户动态余额 |
| `api/internal/usage-ingest.openapi.yaml` | 扩展生命周期/语义批量事件、ACK、结算身份与错误；内部预算 admit 接口与生成 client 纳入本仓库 API 管理，可集中在独立 `budget.openapi.yaml` |
| `api/admin/admin.openapi.yaml` | 五类用户 budgets GET/PUT/清除覆盖、限额模板 CRUD、单模板指派/解除、状态、审计/待核对；usage 增加协议过滤、用户汇总、按日趋势和详细 meters；平台拒绝数与消耗请求数分开 |
| `api/client/client-control.openapi.yaml` | `/api/portal/v1/budgets`、`/usage/summary`、`/usage/trend`、`/usage/requests` 及请求详情；只接受本人 Portal Session，不开放指定 userId，也不下发模板身份/修订/指派元数据 |
| 同一 Client OpenAPI | 增加 `/api/client/v1/budgets` 本人摘要，Client Bearer 认证供 Android 企业空间使用；与 Portal DTO 共享有限/无限、配置来源、状态、多个阻断项及 asOf 等语义，不能将 Portal Cookie 当作原生 credential |
| Runtime Problem | 定义 budget_exhausted（429）、budget_service_unavailable（503）、usage_reconciliation_required（503）、usage_meter_unavailable（422），分别表示耗尽、服务不可用、待核对和当前请求模式无法计量；预算耗尽给出能力/指标/周期及可得 resetAt，准入拒绝使用 forwarded=false；只有可确定整体自动恢复时间才给 Retry-After。已删除身份的 Runtime 与 refresh credential 统一返回 401 `enterprise_identity_deleted` |
| Snapshot v4 | 不增加预算余额、动态规则或模板元数据；新增可选 `imageGenerators` 与 `policy.defaultImageGenerationId`，缺失仅表示空/未设置。新 writer 显式写数组；不升版，不为该字段创建数据库迁移或双读协议 |
| Bridge v3 | 不新增预算方法；Portal 用同源 HttpOnly Cookie 查询。同步生成包，不制造新 token 通道 |

Portal API 在每个 handler 从 authenticatePortal 推导 userId，查询和详情都在 DB 层强制归属；不能只前端隐藏。用户响应去除内部 route、Secret、管理审计及其他用户信息；使用 no-store 和已有来源/会话检查。Admin 权限仍走 Admin API。

Runtime Problem 增加结构化 budget 上下文（能力、资源、阻断指标数组、周期、上限/已用、resetAt 可空、requestId、forwarded=false）；不只给一个 message。多阻断项的整体恢复必须等待所有适用限制解除，累计阻断不返回虚假整体恢复时间。预算耗尽返回 429，但不得复用无界通用 429 自动重试策略。

Android 维护方同步当前生成合同、错误枚举/消费测试；在 LLM 对话、工具续接、TTS 和 ASR 操作位置解释企业名、资源/能力、耗尽项、周期、恢复或联系管理员，并提供 Portal“我的用量与额度”入口，保留已有对话。企业空间打开/返回前台读取 Client 本人摘要，明确显示无限模式，失败不能补无限/零。响应绑定发起时的 Realm/Session，切域取消或丢弃迟到结果。预算错误不得自动 logout/清除绑定或无限重试。

已删除身份不是普通 token 过期：Core 对旧 access/runtime credential 与 refresh credential 返回同一稳定 401 code；Android 按企业错误标准显示“企业身份已删除、需联系管理员或重新绑定”，清理本地企业 Session/Realm 私有状态且保留个人数据，不进入 refresh 重试环或退化为泛化网络错误。

预算中间件仅放企业 Runtime 准入，不能套在 Client/Portal 路由外层阻断空间、grant、查询或同步。门户深链使用现有进入/换票路径与受限固定页面目标，不引入任意 returnUrl 或新 Bridge credential。原生实际入口、上下文提示和切域需 Android 仓库实现并设备验证，本项不能仅凭 Core/Portal 测试宣称跨端完成。

## 7. Admin 与 Portal 页面落地

Admin 在 UsersPage 用户详情增设“用量与额度”：MODEL/TTS/ASR/MCP/IMAGE_GENERATION 五类能力先选择无限/有限，有限模式编辑自然周期及累计规则，显示已用/在途/剩余/恢复、阻断原因和变更历史。图片只提供 REQUESTS/REQUESTED_IMAGES。默认无限、限额模板与用户覆盖由服务端配置来源区分；无限仍展示用量，零为禁用该指标额度。历史图表时间筛选与当前额度卡分开，不提供资源级预算。

一级 `Budget Templates` 位于 Users 与 Resources 之间，复用同一个 `BudgetRuleEditor`。一个用户最多指派一个 live-linked 模板；显式能力覆盖优先，清除覆盖恢复模板/默认，解除模板保留显式覆盖。模板修改在单事务中更新所有指派用户的未覆盖能力；被指派模板禁止删除。模板名称、ID、revision、assignment 和详细来源只出现在 Admin API/UI，不能进入 Client、Portal、Snapshot、Runtime 或 Android。

同一详情提供正式“删除用户”操作：操作者必须逐字输入当前用户名并填写原因，服务端再次核验；删除状态机先 deny 新的 Client/Runtime/refresh 请求，等待或拒绝在途请求，再以单一可恢复事务清除用户、设备、Session、配置、预算、用量、Feed/Portal 与审计外的用户私有事实。审计记录使用不可反查的主体摘要；旧 access/runtime credential 及 refresh credential 以不可逆摘要 tombstone 识别并稳定返回 `enterprise_identity_deleted`。删除失败不得呈现成功，重试保持幂等，Admin 显示最终 COMPLETED/FAILED 与可操作诊断。

UsagePage 提供用户搜索/汇总、协议及资源分布、按日趋势；复用 UsageRequestList 的分页和详情，不另造第二套个人明细逻辑。详情含调用次数、token 等语义、完整度、预算周期和失败原因。常用数值本地化为万 token、字符、分钟；精确值在详情可查。

Portal 增加首页额度摘要及“我的用量与额度”，与 Admin 同源服务计算但为不同权限 DTO。本人可以查看额度、个人趋势、资源分布和分页调用明细。无预算仍显示使用量；累计显示不自动恢复；额度恢复时间来自 Hub，页面不推算自己的时区边界。

新增读取复用 Portal source/session 的取消和用户隔离。打开、回到前台及手动刷新；仅可见且存在在途状态时短间隔刷新，停止隐藏页面轮询。统一 updatedAt/asOf，失败保留明确陈旧提示，401/403 清空私有数据，迟到响应丢弃。小屏、空结果、部分未知、超额、加载失败都要有可操作表现。

## 8. Android 配额错误：所有资源入口共用一个边界

统一实现位于企业请求/错误边界，LLM provider、Speech 和 MCP 入口消费同一个类型化预算错误，不在各模型 UI 重复 JSON 解析。`ModelExecutionService`、`PlatformEnterpriseService`、`SpeechApplicationService`、`EnterpriseVM/EnterprisePage` 及 MCP routed HTTP/WebSocket 路径只负责各自入口编排，可信 Problem 解析与 Realm 失效语义保持单一 owner。

| 发生位置/时机 | Core 必须返回/执行 | Android 必须呈现/处理 |
|---|---|---|
| 点击发送时已经耗尽，或另一个设备刚消耗最后名额 | 在 SSE/JSON/binary/101 开始之前统一返回 HTTP 429、application/problem+json、`code=budget_exhausted`、`forwarded=false`、预算上下文 | 保留用户输入及已有对话；在失败消息/操作位置显示企业、资源、耗尽指标、使用值和恢复时间；提供“查看用量与额度”。不能只显示 rate limit exceeded。 |
| 本轮模型答复成功后 token 才达到/超过额度 | 已准入请求照常完成并按真实值结算，后续模型调用准入返回预算错误 | 不把已成功答复改成失败；调用终结后刷新本人预算摘要，若结算仍在途显示处理中并有界刷新。达到额度后给出下一次调用将受限的提示。 |
| 工具执行后模型续接被拒绝 | 保留同 interactionId 下前序请求事实，拒绝本次新的模型调用 | 明确“工具已执行，但后续模型回复因企业 LLM 额度耗尽未发起”，保留已有工具结果；停止自动续接，不自动重复工具副作用。 |
| TTS/HTTP ASR 额度耗尽 | 在发起上游前返回同一 Problem | TTS 提示朗读额度；ASR 提示转写额度，停止本次执行、释放录音/网络资源，保留已识别内容。不能自动改用个人凭据或其他企业绕过限制。 |
| Realtime ASR 建连时耗尽 | HTTP 升级之前返回 429 Problem，不发 101 | WebSocket onFailure 保留握手响应状态和有界错误 body，转换同一预算异常；不能只报“WebSocket 连接失败”。 |
| 已建立实时会话中额度达到 | 本阶段按已准入会话允许完成，受单次时长上限保护，不插入伪造供应商预算事件；新连接被拒绝 | 不虚构流内 429 或误称握手被拒绝；结束后刷新额度。因运行时长保护关闭与额度拒绝使用不同说明。 |
| 上游自己返回 429 | 保留上游错误，不能给它盖上平台 `budget_exhausted/forwarded=false` 标记 | 仅在受信企业路由的结构化平台错误匹配时走预算提示；普通供应商限流保留原分类，不得说用户额度耗尽。 |
| 无限预算/摘要加载失败/预算服务中断 | 分别返回 UNLIMITED、读取错误或独立 503 预算错误 | 分别显示无限及已用量、暂不可获取可刷新、额度服务暂不可用；不会将任一状态伪装成额度耗尽，也不自动 logout。 |
| 有限额度所需指标不支持当前请求格式 | 返回 422 `usage_meter_unavailable`、受限指标及可解释的格式/模式原因，未转发 | 提示“当前音频格式无法用于企业时长额度，请使用支持的录音格式”等操作指引；不是“额度已用完”，不自动换个人资源。 |
| 用户已被管理员删除，旧 access/runtime 或 refresh credential 再使用 | 返回 401 `enterprise_identity_deleted`，不转发、不刷新、不泄漏删除前资料 | 停止 refresh/retry，按企业标准错误展示明确原因；清理企业 Realm/Session 私有状态并保留个人空间与本地个人数据，提供重新绑定或联系管理员动作。 |

HTTP 错误必须先于各 provider 的 SSE/JSON/audio decoder 统一处理；错误 body 只读一次、有大小限制且保留结构化字段，不能先包装成泛化字符串后再正则抽字段。取消和普通网络异常保留原语义。成功 HTTP/SSE 不依赖在结束后补写 header 来通知额度，使用本人摘要查询。

Core OpenAPI 和共享 fixture 至少定义 `BudgetContext` 的语义字段：当前能力/资源、mode、多个 blockingLimits（meter、period、limit、used、resetAt 可空）、整体 resetAt 可空、asOf；Problem 继续带 requestId/forwarded。数值单位与 meter 一致，AUDIO_SECONDS 明确秒；内部毫秒不泄漏为同名秒。错误显示名优先 Android 受信 Realm/资源目录，缺少名称也保留本地化能力名和服务端说明，不依赖泄漏内部 upstream 信息。

示例提示：“华东研发企业 · DeepSeek · 今日模型调用已达 100/100 次。明日 00:00 恢复，本次请求未发送。”多个阻断项可展开；累计额度提示不自动恢复。出现预算拒绝立即刷新企业空间额度；对同一次错误不同时弹多个对话框/Toast。用户取消提示后仍可进入空间、Portal、同步配置和使用其他可用能力。状态绑定发起时的 Realm、用户和 Session，切域后丢弃旧请求更新。

## 9. 一个完整任务，按仓库串行完成

本项作为一个持续推进的完整任务：架构 → Core（含 Admin）→ 企业门户 → Android → 最终联调。各仓库内部按依赖顺序实现全部内容，不再按“先 Chat 再跨端、再其他协议再跨端”拆分交付，不为每协议、每页面建立独立阶段或等待用户逐项批准。单一任务可以有内部清单，只有仓库完成检查点和最终联调检查点。

按用户约定，每仓库本轮全部实现与测试用例编写完成后做一次集中验证，再进入下一个仓库；不在每个文件/协议完成后重复跑整套检查。本任务不要求逐小改动执行 Red/Green gate。集中验证可包含多个必要测试/构建命令；失败时修复并重跑受影响验证，不将“一次”理解为失败也不重试。新发现的真实回归才触发已通过范围的复验。

| 顺序/仓库 | 一次完成的实现内容 | 本仓库集中验证与交接 |
|---|---|---|
| 1. `measix-architecture` | 补充合同、边界、全部协议最小范围、有限/无限、错误上下文、各端职责、不升版及唯一执行顺序 | 全部文档完成后检查引用、冲突和验收覆盖；交接完整合同，不留“稍后再定义”的必需产品语义。 |
| 2. `measix-platform-core`（包含 Admin） | 一次实现 §3 全部生产解析、Image Generation、spool/账本、五类能力的所有周期预算/恢复、live-linked 限额模板与覆盖、Admin 全部页面与分析、Client/Portal 全部接口、Runtime Problem、初始化 SQL/Ent、fixtures/生成包、运行手册。预算 API 和 DTO 一次交齐给两端 | 集中运行适用 Go/契约/生成一致性、全部新增预算/协议故障测试、Console 测试与 production build、真实 Hub/Relay/Admin 浏览器流程。协议客户端直接验证两种受限查询认证及所有错误 fixture，供尚未改造的 Portal/Android 对接；不提前把模拟客户端当 Android 验收。固定 Core 提交/构建和接口产物。 |
| 3. `measix-enterprise-portal` | 一次完成首页额度卡、个人额度/趋势/分布/明细、有限/无限/多阻断/待结算、深链、移动端及 Session 失效/请求隔离；消费步骤 2 的全部合同 | 完成后集中 generation/typecheck/test/build 和浏览器 E2E，连接已完成 Core 验证实际本人查询/权限；固定 Portal 构建。此时不为 Portal 临时 invent 新 DTO 或复制后端计量。 |
| 4. `rikkahub_mcp`（Android） | 一次同步 wire/fixtures、include_usage 请求构造、全部 LLM/Speech/MCP/WS 的统一预算错误、企业空间摘要、调用后刷新、Portal 用量入口、无限和 Realm 隔离 | 全部代码完成后集中 JVM/契约/适用 UI 测试、编译/lint及需要的设备测试；使用已完成 Core 与 Portal。固定 APK/提交；不只以 LLM Toast 完成代替所有资源入口与空间状态。 |
| 5. 最终跨端联调（同一任务） | 使用上述同一组 Core/Portal/APK，在 Admin 配置用户与额度，从 Android 进入企业空间、调用真实资源并查看 Portal | 一次集中覆盖下节全部联调场景，收集各仓库提交/构建、测试结果和真实供应商证据；修复联调缺陷并复验相关链路，全部满足才关闭任务。 |

上游合同发现确需更改时回到 owner 修正并重生成受影响消费者；这是缺陷修复，不重启多个协议轮次。不递增 Snapshot/Bridge/API 版本。Core 集中验证应提前覆盖下游需要的所有错误/无限/多周期 fixture，降低后续回改概率。运行手册补齐 Hub 准入依赖、spool/待核对监控和审计核对；开发期直接重建本任务相关本地测试数据库与持久化，按 §1 已授权范围执行，不做迁移。

## 10. 最终联调必须证明的用户闭环

1. Admin 为用户设置 LLM 日 100 次/1,000 万 token，两次独立场景分别耗尽次数与 token；测试环境用较小额度完成边界，界面保留同单位语义。Android 实际拒绝前上游调用数不增加，提示上下文完整，Admin/Portal/空间数据一致。
2. token 在当前成功回复后才达量时保留成功内容；下一次发送、工具续接、多设备竞争和 SSE 建连失败走 §8 的不同提示；供应商 429 不被误判。
3. 日/周/月及累计规则先到即停、跨周期在途结算、多限制恢复时间、有限/无限切换保留历史；无限用户仍有明细，不受额度拒绝影响。
4. TTS 字符、HTTP ASR WAV 时长、实时 ASR 会话与音频、MCP 次数分别用实际生产解析交付；SYSTEM_TTS 不伪造服务端量。各协议正常/取消/失败均有准确来源与完整度。
5. Admin 查询不同用户和资源分布，Portal 只见本人；翻页、趋势、空/错误/陈旧状态和小屏操作实际可用。Admin 用户汇总的服务端额度筛选按待核对、耗尽、接近上限、可用取最高优先级；接近上限使用已用加预留达到 80% 且尚未耗尽的展示阈值，不改变准入。耗尽后 Android 空间、Portal 查询、配置同步和其他能力仍可用。
6. Hub 暂不可用、Relay 重启、重复/乱序结算、usage 缺失及待核对恢复不重复扣减、不静默放行；有界流式解析及 WebSocket 分片不破坏传输。
7. 真实 Android 验证 Portal 进入/深链、企业提示、Session 到期和 Realm 切换迟到结果；不能以浏览器 native bridge 替身、固定 usage 或仅文档勾选代替。
8. Admin 错误确认不能删除；正确用户名与原因启动 deny-first 全量清理，在途调用不逃逸。删除后该用户的设备、Session、配置、预算、用量和 Portal 私有数据均不可查询，旧 access/runtime 与 refresh credential 都稳定返回 `enterprise_identity_deleted`；Android 显示标准企业身份删除错误并只清理企业域状态。
9. Admin 创建并编辑五类限额模板、指派用户、覆盖与清除单能力、更换/解除模板、验证被指派模板无法删除；模板变更实时作用于未覆盖能力且不清零同周期已用量，Client/Portal/Android/Snapshot 均无模板元数据。
10. Admin 发布真实 `img_*` 资源和可选默认项；未设置默认保持未设置。Android 从 Snapshot v4 选择企业文生图资源，经 Relay 实际返回 URL 或 `b64_json` 并由 GeneratedMediaStore 接管；图片请求数与请求图片数分别达到额度后 no-forward。

每个声称已验证的供应商/profile 保存真实证据；无法获得凭据/设备时保留具体未验证项，继续可完成的仓库工作，最后不得宣称整个任务验收通过。测试、build、供应商资格及 Android 设备证据分开记录；统计与预算同一 authority，但不声称所有兼容供应商提供所有指标。
