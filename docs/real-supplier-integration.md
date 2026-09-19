# 真实供应商接入与验证记录

日期：2026-09-19。本文记录这台开发环境四组凭据的实际结果，以及 S0.1/S0.2 已发布给 Android 联调的资源组合。密钥只在 Git 忽略的 `.secrets/supplier-keys.env` 和 Hub Secret Store 中，不能出现在 Snapshot、导出包、用量记录或本文。供应商能力、账户授权和 Core/Android 协议覆盖分别判断；HTTP 可达性探测不等于资源调用成功。

## 逐项结论

| 供应商 | 真实请求与结果 | S0.1/S0.2 发布选择 |
| --- | --- | --- |
| DeepSeek | `deepseek-flash` Chat Completions 文本、流式、强制工具调用及工具结果后续回合、图片理解均返回 HTTP 200；经隔离 Core Relay 的模型 normal/stream/cancel/timeout/auth/error 五项通过 | 发布为 OpenAI Chat Completions，输入 TEXT/IMAGE、输出 TEXT、TOOL 能力；使用 Bearer Secret，客户端只见稳定资源 ID 和 Relay 路径 |
| 小米 MiMo | `mimo-v2.5-tts` 按 MiMo Chat Completions TTS 生成真实 WAV（115244 字节），隔离 Core Relay 的正常/流式/取消/超时/错误边界五项通过；当前账户请求 `mimo-v2.5` 文本与 `mimo-v2.5-asr` 返回 HTTP 402 | 只发布 `MIMO_CHAT_COMPLETIONS_TTS`，`api-key` 静态 header；另发布无上游的 `SYSTEM_TTS` 供设备选择。当前不发布 MiMo LLM/ASR |
| Firecrawl | 带 Bearer 的 hosted Streamable HTTP MCP `initialize`、`notifications/initialized`、`tools/list`、`firecrawl_scrape(example.com)` 均成功；经隔离 Core Relay 的 initialize/list/call/session/cancel/error 六项通过 | 发布 Direct MCP `MCP_STREAMABLE_HTTP`，企业 Secret 属于 Upstream，`authOwnership=NONE`，企业助手引用此 MCP |
| 阿里云百炼新加坡 Token Plan | 用户指定的 `token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1` 直连：`qwen3.8-flash` 对话、工具调用及后续回合、图片理解、`qwen-audio-3.0-tts-plus` 音频、`qwen-audio-3.0-asr-flash` 文件识别、`qwen-image-3.0-pro` 生图均 HTTP 200。工具具名强制调用必须传 `enable_thinking=false`；默认思考模式下返回 400。先前对北京或按量付费 URL 的 401 是地址/计费方案不匹配 | 用户明确本机仅做实验，已将密钥保存到此开发 Hub 的 Secret Store 并建立实验 Upstream；`DASHSCOPE_HTTP_ASR` 现有明确协议及测试，可在 Android 实现同一协议后用于设备联调。官方禁止 Token Plan 用作真实后端服务，不能把本机实验配置直接运营化；生图仍不属于 S0.1/S0.2 托管资源 |

DeepSeek Chat Completions 和工具调用以[官方接口](https://api-docs.deepseek.com/api/create-chat-completion/)与[工具说明](https://api-docs.deepseek.com/guides/tool_calls/)为准；MiMo TTS 的音频消息格式及 `api-key` 见[官方指南](https://mimo.mi.com/docs/zh-CN/quick-start/usage-guide/audio/speech-synthesis-v2.5)。MiMo [402 说明](https://mimo.mi.com/docs/en-US/api/guidance/error-codes)是账户余额不足，不能靠改协议处理；当前 TTS 免费阶段仍可成功。Firecrawl hosted MCP 地址与认证见[官方说明](https://github.com/firecrawl/firecrawl-docs/blob/main/mcp-server.mdx)。MCP 通知没有请求 ID 就不应产生 JSON-RPC 响应；无 ID 的 `notifications/initialized` 应返回 HTTP 202 且无 body，见[MCP Streamable HTTP 规范](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)。百炼[官方 Base URL 总览](https://help.aliyun.com/zh/model-studio/base-url)明确地域/计费方案密钥不可混用及 Token Plan 后端使用限制；[函数调用说明](https://help.aliyun.com/en/model-studio/qwen-function-calling)解释具名强制调用需要关闭思考模式；[HTTP ASR 官方接口](https://help.aliyun.com/en/model-studio/fun-asr-flash-recorded-speech-recognition-http-api)定义 JSON Data URI 和 `output.text`。

## 当前环境发布组合

公共入口 `http://192.168.31.235:9100` 同时承载 Discovery、Client API、Runtime Relay、Portal 和 Admin；手机在同一局域网可使用 HTTP IP 地址。当前第 5 版下发 DeepSeek Flash 和 Qwen 3.8 Flash 两个 TEXT/IMAGE/TOOL 模型、MiMo 云端朗读与设备系统朗读、百炼 HTTP 录音转写、Firecrawl Direct MCP、两个企业助手和三个常用入口。`policy.defaultModelId` 指向 DeepSeek，`defaultTtsId` 指向 MiMo，`defaultAsrId` 指向百炼，`defaultAssistantId` 指向企业工作助手。五项 `allowLocal*` 允许在企业域按策略混用个人资源。Qwen 和百炼 ASR 标注“本机实验”；百炼 TTS 与生图已直连验证，但尚无 S0.1/S0.2 对应的客户端托管 profile，因此没有伪装成可下发的 TTS/生图资源。

第 5 版经同一公共入口的 Client Session 刷新、Snapshot、应用回执及 Runtime Relay 实测：DeepSeek 文本 SSE 的 `finish_reason` 先于 `[DONE]`、工具及后续回合、图片理解均 200；MiMo WAV 和流式 PCM 成功；Firecrawl initialize/list/scrape 200、无 ID 通知 202 空 body；Qwen 文本、工具、后续回合、看图均 200；百炼 ASR 200 且返回 12 字转写。MCP 路由最初只允许 POST，历史用量里的 `ROUTE_POLICY_DENIED` 403 是 Core 自身阻断；第 5 版补齐 POST/GET/DELETE 后，GET 到达上游返回 405，DELETE 无会话标识到达上游返回 400，均不是 Core 403。管理员 Upstream 的 HEAD `401`/`404` 只反映服务根路径响应，必须用具体资源请求检查协议、认证、音频和 MCP 工具结果。

## 可复核的证据与限制

隔离资格脚本 `scripts/collect-adapter-qualification.mjs` 使用被忽略的 `.env.adapter-qualification` 调用真实供应商。最新 `.artifacts/real-adapter-qualification.json` 记录模型 5、TTS 5、MCP 6 个真实转发请求；用量记录 16 条、转发 16 条，按资源归属 16 条。该报告未纳入本次百炼 ASR，且 SaaS Adapter 构建版本无法按资格规格验证，因此整体状态仍为 `FAILED`，不能作为正式 S0.1 Freeze。第 5 版公共环境的资源级 Usage 已记录 DeepSeek 27 次、MiMo 6 次、Firecrawl 43 次（其中历史 Core 路由阻断 7 次）、Qwen 8 次、百炼 ASR 2 次；五类都有真实转发。当前 Upstream 为 `LEVEL_0`，请求数与资源归属可验，供应商 token、费用和请求语义完整度仍为未知，不能把“未知”当作零。以上计数是 2026-09-19 本次检查时的快照，会随联调增长。

合成 Adapter 的 Chat SSE 现先发带 `finish_reason=stop` 的终止 chunk 再发 `[DONE]`；MCP 对无 ID 初始化通知返回无 body 的 202。相应 Go 测试和 9100 公共入口实测通过，但合成数据不计为供应商能力。第 5 版已完成公共入口协议客户端的真实供应商调用。Android 当前源码已有 `DASHSCOPE_HTTP_ASR` 的候选映射、WAV Data URI 编码和 `output.text` 解析；设备级消费仍需另行确认。

2026-09-19 的 Android 模拟器一次文件识别请求经 Core 成功转发至百炼，但上游返回 400；Usage 记录请求体约 631.5 KiB，模拟器日志显示 `HTTP 400: {}`。用同一第 5 版资源和 Client Session，按 Android 源码构造的 `parameters={format:"wav",language_hints:["zh"]}`，以及去掉语言提示、补 `sample_rate:"24000"` 的两种对照请求，均通过真实 Relay 返回 200 和 `output.text`。将同一 WAV 结构的音频改为 8 秒纯静音 PCM 后，百炼返回 400，复现设备看到的空错误响应。由此已排除 Hub 路由、凭据和 `language_hints` 作为该失败的直接原因；模拟器实际录音是否全静音仍须设备侧核查（例如录制期间 RMS、WAV 头和 PCM 非零样本数），再用有声样本复测。不能把这次模拟器调用计为 ASR 已通过，也不能把协议客户端的成功冒充为设备验收。[百炼官方音频规格](https://help.aliyun.com/en/model-studio/asr-model)说明该模型接受 WAV、任意采样率和 5 分钟以内的单段录音。
