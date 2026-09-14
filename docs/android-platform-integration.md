# Android 真实平台接入说明

本说明面向 Android 维护方。当前唯一组合是 Discovery protocolVersion="1"、Snapshot v4、Portal Bridge v3、localReadVersion=2；资料包 formatVersion=1 独立。MEASIX 从未发布，不做旧 Snapshot、旧数据库或配置转换。Android 0.0.20 的本地消费者可复用，但 `LocalEnterpriseSource` 仍拒绝 Platform 接入材料；本说明及上游测试不代表真实平台 source 或设备联调已完成。

## 权威与资料入口

- [Control Protocol](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md)：§8 认证、§10 配置、§11 Runtime；协议语义有异议时回到此处。
- [Experience Contract](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-enterprise-realm-experience-contract-spec.md)：企业配置与用户数据边界。
- [阶段索引](../../measix-architecture/docs/measix-stage-document-index.md)：S0.2 范围与下游测试入口。
- [Android 可执行 OpenAPI](../api/generated/android/client-control.openapi.yaml)：已展开外部依赖；HTTP 请求、响应、enum 的唯一可执行结构来源。
- [共享接入资料](../api/fixtures/client-integration/cases.json)、[HTTP 时序示例](../api/fixtures/client-integration/http-examples.json)、[完整 Snapshot](../api/fixtures/client-integration/snapshot-v4.json)、[全部禁止策略](../api/fixtures/client-integration/snapshot-v4-denied.json)、[语义反例](../api/fixtures/client-integration/reference-cases.json)。
- [接入材料正反例](../api/fixtures/enrollment/cases.json)、[原生 Bridge 正反例](../api/fixtures/portal/native-vectors.json)、[Feed 正反例](../api/fixtures/portal/feed-vectors.json)。

资料由 `cmd/generate-client-fixtures` 调用真实 Snapshot compiler 生成，输入 `api/fixtures/draft/s02-client-profile.json` 是公开资源投影配方，bindings 为空，不是可直接发布的运营草稿。真正发布还需管理员配置私有 Upstream/Secret/Binding 并通过发布验证。合成令牌、接入码、时间和 ID 只用于测试，不能用于服务器登录。Canonical hash 由 core 当前 `capability.HashSnapshot` 复算验证；客户端按 Control Protocol §10.13 校验 ETag/body.snapshotHash 一致性，不另造 JSON canonicalization。文件 SHA-256 与 Snapshot hash 是不同概念。

独立包 `api/generated/android/integration` 包含本说明、所需 schema/fixtures 和架构文档。把整个目录复制出去即可执行 `node verify.mjs --verify .`；不需要兄弟仓库。manifest 摘要覆盖原始文件字节，任何漏文件、额外文件、修改或非法路径均失败。包是工作树候选，不能将 sourceHash 当成 Git commit。

## Android 消费模型映射

以下 Kotlin 名称只用于定位当前实现，不改变 wire。平台 source 应有独立适配层，复用现有 ConfigurationResolver/执行准入，不能修改本地示例 source 来伪装平台已经接通。

| 平台字段/事实 | 当前 Android 模型处理 |
| --- | --- |
| 已验证的 HTTPS origin + deploymentId | 绑定 EnterpriseAuthority。规范化 origin（scheme/host/port），保存可信平台来源；sourceNamespace 使用现有 `platform:` 命名空间机制。展示名、令牌轮换不改变来源；另一个 origin 不能因同名/同 deploymentId 自动合并 |
| userId/deviceId/sessionId | 用户域归属与认证会话分别保存；重登可以同 user/device，但必须采用服务端新 Session，不复活旧文档或旧任务 |
| managedGeneration / releaseId / snapshotHash | generation 映射 EnterpriseConfiguration.generation；同时保存用于一致性验证的 release/hash，整个候选验证后原子替换 |
| Provider.providerId/displayName/clientProtocol/enabled | 适配层保留 Provider 与协议元信息；现有 EnterpriseModel 没有完整 Provider 表，不能丢失后猜测协议。禁用 Provider 下的资源不可执行 |
| Model.modelId / upstreamModelKey | 前者是稳定平台资源 ID，映射 EnterpriseModel.id；后者映射其请求模型名 modelId，写到请求 body 的 model。不能互换 |
| Model.displayName/modalities/capabilities | name 与显式 enum 映射。S0.2 为 CHAT；不映射成图片生成/附件等未声明 profile。未知 enum 拒绝候选，不能默认为普通文本模型 |
| Provider.clientProtocol + Model.runtimePath | 保存在平台运行适配配置中，决定公开 Relay URL 与请求 profile；不塞入本地私有 binding，也不向用户存储注入企业上游地址/密钥 |
| TTS.ttsId/displayName/upstreamModelKey/voice | EnterpriseTtsResource.id/name/modelId/voice；保留 clientProtocol/runtimePath，使用平台 access token |
| ASR.asrId/displayName/upstreamModelKey/language | EnterpriseAsrResource.id/name/modelId/language；未指定 language 保持空，不猜测语言；仅当前 HTTP transcription profile |
| MCP.mcpServerId/displayName/enabled | EnterpriseMcpResource.id/name/enabled；适配层保留 MCP_STREAMABLE_HTTP、runtimePath、ENTERPRISE_MANAGED；使用 Relay，无 OAuth/上游凭据下发 |
| Assistant.assistantDefinitionId/displayName/description/modelId/systemPrompt/mcpServerIds/enabled | EnterpriseAssistant 对应字段；缺省 description 可展示为空字符串；引用的 modelId 是平台稳定 ID，不是请求模型名 |
| Assistant.memorySeed[] | 按作者顺序转换为只读 Seed。内部 ID 从当前 authority、助手 ID、generation、索引确定；空数组有效，条目不得为空白。不按内容去重、不建立可变 Assistant Memory 副本，配置替换时整体换代 |
| Starter.starterId/assistantDefinitionId/title/prompt/description/sortOrder/enabled | EnterpriseStarter 对应字段；仅启用且助手有效的入口可操作。展示按 sortOrder、starterId 排序。点击只进入原生输入草稿，由用户发送，不新增 Portal 聊天写入 Bridge |
| policy 五项 allowLocal* | EnterprisePolicy 五项必填 Boolean；缺失/null/错误类型均拒绝。只控制本企业域内用户原配置准入，不复制用户定义、端点和密钥 |
| defaultModelId/defaultTtsId/defaultAsrId | defaults.chatModelId/ttsId/asrId；显式无效引用不回退首项。未提供的默认值保持未指定，由既有本域选择规则处理 |
| defaults.assistantId/fastModelId/titleModelId/imageGenerationModelId/attachmentInspectionModelId/suggestionModelId/compressModelId | v4 无对应的企业强制字段。保留本域偏好和功能已有选择规则，不把 defaultModelId 批量写入所有槽位，不继承本地示例值 |
| allowAsSubAssistant/allowedSubAssistantIds、gateways | v4 不下发企业子助手关系或 Gateway；平台适配输出 false/空集合。用户自有子助手仍按现有五项准入和执行权限处理，不扩展 wire |

## 认证、同步与恢复时序

1. 扫码或粘贴只解析共享 EnrollmentMaterial；明确 sourceKind，再验证 HTTPS 来源与 Discovery。校验 product、protocolVersion、deploymentId 和当前支持版本，未知来源不得借本地域配置绕过。
2. `POST /api/client/v1/enrollments/exchange` 使用一次性 code、稳定 installationId、deviceName、appVersion、ANDROID；成功 201。原子保存整组 access/refresh、各到期时间、身份。失败不建立半个企业域。
3. access token 只用于 Client/Runtime Bearer；Portal JS 不可读取。`GET bootstrap` 的 session.expiresAt 是会话 idle 到期时间，不能替代 accessTokenExpiresAt。七天 idle 只由有效刷新续期，普通查询不续期。
4. 没有首个 Release 时 activeManagedGeneration=0，保持待配置且 Runtime 禁止；不请求 Snapshot 0、不因 READY 字样而执行。首次 Bootstrap 不带已应用 generation，因此有发布配置时也需要同步。
5. `GET managed/state` 可带 `X-Measix-Applied-Managed-Generation`。下载目标 generation 的 Snapshot，验证部署/版本/generation、ETag 与 body.snapshotHash 一致性、必填策略、资源 enum 与引用闭包后原子应用，之后才报告已应用 generation。保留最后完整状态用于展示，但不能绕过新状态的执行阻止。
6. Snapshot HTTP 200 返回 JSON 与带引号的 ETag；If-None-Match 命中返回无 body 的 304。仅当同一来源、身份及 generation 的已验证缓存存在时才能复用。真实 HTTP 304 与 Portal 本地消息响应是两套语义，本地消息不得伪造 HTTP 304。
7. 每个 Session 的 refresh 串行化。先持久化本次 Idempotency-Key；不确定响应只用同一旧 refreshToken + 同一 key 重试。服务端短恢复窗内返回完全相同的轮换结果；原子提交新令牌后才开启下一次刷新。旧 token + 新 key / 新 token + 旧 key 为 409 refresh_conflict。恢复窗已过按明确失败重新接入，不循环重试。
8. Runtime 请求冻结 authority/session/generation/resource；`428 managed_snapshot_required` 且 forwarded=false 表示未转发，进入配置同步与重新准入，不能绕过 generation barrier 或盲重放已经发生 I/O 的业务。
9. Client logout 是 `POST sessions/logout`，JSON body 携带当前 refreshToken；不是 access-token-only 请求。成功 204。母 Session 撤销后 Client 查询返回 403 session_revoked，正确 ETag 也不能绕过授权。401 认证过期/失效与 403 撤销、资源禁止分别处理。
10. 原生确认取消保留原状态；确认后由持久 owner 完成退出。先撤销旧文档交付、取消/等待原生采集与写入，再清理 Realm/Portal 站点数据和媒体。站点清理失败可重试，完成前不得展示新 Realm。旧接收器、旧媒体句柄、旧请求不能写回新页面。

## Runtime 请求示例

从可信 origin 拼接 Discovery.runtimeApiBase + `/resources/` + **稳定资源 ID** + 该资源 runtimePath。示例 origin 为 `https://platform.example.invalid`。不能把 upstreamModelKey 放在 URL 资源 ID 位置，不直接请求 Upstream，不用 Portal Cookie 调 Runtime。

每个请求带 `Authorization: Bearer <accessToken>`、`X-Measix-Managed-Generation: 42`、`X-Measix-Interaction-Id: <本次合法 interaction ID>`。后两个值由原生执行上下文冻结；Managed State 查询使用的 Applied header 不能替代 Runtime header。

| profile | 示例 POST URL 后缀 | 请求与响应 |
| --- | --- | --- |
| OPENAI_CHAT_COMPLETIONS | `/runtime/v1/resources/mdl_bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb/v1/chat/completions` | JSON `{"model":"gpt-4o","messages":[{"role":"user","content":"你好"}],"stream":true,"stream_options":{"include_usage":true}}`；SSE 按事件读取，取消关闭响应流。非流式使用 stream=false。tools/tool_calls 和图片内容依照声明 profile，由客户端 SDK 构造；Relay 不翻译 body |
| OPENAI_AUDIO_SPEECH | `/runtime/v1/resources/tts_cccccccc-cccc-4ccc-8ccc-cccccccccccc/v1/audio/speech` | JSON `{"model":"tts-1","voice":"alloy","input":"你好"}`；读取二进制音频，不当 JSON/SSE 解析 |
| OPENAI_AUDIO_TRANSCRIPTIONS | `/runtime/v1/resources/asr_dddddddd-dddd-4ddd-8ddd-dddddddddddd/v1/audio/transcriptions` | multipart/form-data：model=whisper-1、language=en、file=<录音>；由 HTTP 库生成 boundary。解析 transcription 响应；不是 WebSocket ASR |
| MCP_STREAMABLE_HTTP | `/runtime/v1/resources/mcp_eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee/mcp/v1` | 标准 MCP Streamable HTTP 会话：initialize → initialized → tools/list → tools/call；Accept 同时允许 application/json 和 text/event-stream。沿用当前 MCP 客户端维护返回的会话/协议 header，不自创固定服务端 Session，不经 Gateway |

完整 428 body 使用 [现行 Problem 样例](../api/fixtures/problem/managed-snapshot-required.json)。这些是请求构造示例，不声称合成 profile 已取得真实供应商资格认证。

## 验证职责与交接清单

上游已具备的测试入口：`contract` 读取同一共享 schema/fixtures 并复算 Snapshot hash；`httpapi.TestClientIntegrationPendingSnapshotAndLogout` 使用真实身份/SQLite/HTTP 验证 pending、应用后状态、200/304、404 与撤销；`identity` 验证轮换幂等、并发、重建服务后的恢复、idle 与退出隔离；`relay` 验证 generation barrier、transport、取消和流式转发。`scripts/export-client-integration.test.mjs` 把包复制到独立目录执行摘要校验并注入篡改/多余路径。

Android 维护方下一步：实现独立 Platform source 和安全令牌/刷新 owner；消费同一资料组补完整映射与候选原子应用；接入真实 Relay 四 profile 和 typed 428；复用现有 Portal/退出 owner 验证站点清理和媒体生命周期；最后用本次包的 manifest/sourceHash 关联真实 Hub、设备、原生构建证据。原生权限确认、系统 WebView、进程死亡、相机/麦克风和真实平台端到端验证属于 Android，浏览器 Mock 不替代这些门禁。
