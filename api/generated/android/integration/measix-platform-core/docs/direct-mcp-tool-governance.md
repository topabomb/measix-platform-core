# Direct MCP 工具治理实施与 Android 对接

语义权威为同级架构仓库的 Control Protocol §10.7.1（源仓库：`measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md`） 和 Admin Product Requirements §9.5（源仓库：`measix-architecture/docs/10-runtime-foundation/s0/measix-s0-admin-console-product-requirements.md`）。本文件维护 Core 实现、操作与消费者接线；版本扩展见[响应消费规则](android-platform-integration.md#响应扩展与适配规则)。当前编制使用 Snapshot v5，已发布 v4 保留独立 DTO、原始 bytes/hash 和重新发布路径。

## 实现归属

| 位置 | 职责 |
| --- | --- |
| `api/admin/admin.openapi.yaml` | 草稿工具许可、完整审核定义、只读发现记录和发现命令 |
| `api/client/client-control.openapi.yaml` | v5 资源许可及助手绑定、独立 v4 wire |
| `backend/internal/hub/capability/mcp_discovery.go` | Streamable HTTP 发现、完整 Tool 的 JCS 摘要和有界传输 |
| `mcp_governance.go`、`mcp_json.go` | 来源指纹、发现 CAS、保存防伪、缺失/null 区分及发布校验 |
| `mcp_projection.go`、`snapshot.go` | Client 投影、排序、发布差异和 v4 保全 |
| `backend/internal/hub/httpapi/admin_capability.go` | Admin Session/CSRF 与稳定 Problem 映射 |
| `console/src/components/McpToolsEditor.vue` | 服务器工具许可、发现、确认策略和合同重审 |
| `AssistantMcpToolsEditor.vue`、`AssistantMcpToolPicker.vue` | 助手服务器绑定及工具子集选择 |
| `McpToolSummary.vue`、`McpToolDetailDialog.vue` | 共用紧凑行及只读完整定义详情 |
| `console/src/stores/draft.ts` | 唯一草稿状态、保存/发现与迟到响应保护 |

工具许可和发现证据存于现有 Managed Draft JSON；Release 保留审核定义，Client 投影只包含许可的 name/hash/approvalPolicy，排除定义、发现身份、来源指纹、上游地址和 Secret。不增加数据库表或改写既有 Release。发现时间和未选候选不计入发布差异。

## 发现与保存

`POST /api/admin/v1/draft/mcp/{mcpServerId}:discover` 使用 `expectedDraftRevision` 和 CSRF。普通上游必须已应用当前配置；工作区来源使用已开通、已连接的用户身份。Hub 从真实 binding 和私有 Secret owner 取得连接，浏览器不能提交 endpoint 或凭据。

发现执行 initialize、notifications/initialized、分页 tools/list 和会话关闭，不执行 tools/call。仅支持 Streamable HTTP 的 JSON/SSE 响应，不支持旧 HTTP+SSE 传输或回退。请求协议版本 `2025-11-25`，接受该版本及 `2025-06-18`、`2025-03-26`，后续请求和会话关闭携带协商版本与 Session ID；支持集合由 `SupportedMcpProtocolVersions()` 返回。SSE 中的服务端 ping 返回空结果，未声明的能力请求返回 Method not found 后继续等待目录。[MCP 传输规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) 和 [ping 规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/ping) 定义这些交换。

总超时 20 秒、响应头超时 10 秒、会话关闭最多 2 秒；目录最多 64 页、4096 工具，响应累计最多 4 MiB。完整成功才替换目录；重复名称/JSON key、循环游标、缺失 tools capability、不完整分页或非法 Tool 均拒绝。远端正文、私有 URL 和凭据不进入 Problem。

不受支持的协商版本单独返回 `mcp_discovery_version`，Problem 的 `receivedMcpProtocolVersion` 只接受合法日期，`supportedMcpProtocolVersions` 来自上述支持集合；其他无效响应或目录仍返回 `mcp_discovery_protocol`。这些字段仅存在于错误响应，不加入草稿、Snapshot 或数据库。界面只在已有成功目录时显示保留提示。

工具摘要使用完整原始 Tool JSON 的 RFC 8785 JCS UTF-8 bytes，经 SHA-256 编码为 `sha256:` 加小写十六进制。description、schema、annotations、`_meta` 和扩展均参与；对象 key 重排不改变摘要，数组顺序保留。JSON-RPC 信封及分页元数据不参与。

网络 IO 不持事务。完成后在短事务中重查草稿 revision、来源配置及认证版本，再以 CAS 更新草稿。失败或迟到结果保全原目录、许可和 revision。普通 Save 必须回传服务器的只读发现记录；新增或重审许可必须匹配该来源的发现证据，旧批准项可保留以展示 drift。Validate/Preview/Publish 共用来源、合同和引用校验；停用服务器保留结构合法的名单，重新启用时恢复审核要求。

## Admin 操作与草稿保全

有修改时“保存草稿并发现工具”先保存整个草稿，保存失败停止发现。期间新增编辑会保留并要求再次保存；保存/发现响应只更新已提交基线及服务器证据，不覆盖后续本地修改。重新加载使旧请求失效；加载期间新增编辑保留旧基线并提示冲突，防止直接覆盖其他管理员的新 revision。

工作区连接选择位于取得目录的操作中。点击发现后，`GET /api/admin/v1/remote-workspace/services/{workspaceServiceId}/workspaces?discoveryEligible=true` 只列出当前有效服务中、用户有效且已连接并持有 MCP 认证绑定的工作区；过滤先于分页，支持显示名、用户名及远端用户名搜索，不依赖 MCP 已发布。唯一连接自动使用并显示名称，多连接使用 `PagedEntityPicker`，没有连接时提供管理入口。选择仅存在于组件内存，成功发现沿用现有 `toolDiscovery.userId`，后端仍复验来源和凭据；助手运行时继续使用各用户自己的工作区。全部工具模式无需发现即可发布，不增加持久化字段或数据库迁移。

服务器 ALL 默认收起只读目录；查看、搜索、发现不编制许可。只有明确切换 ALLOWLIST 后显示选择和确认设置。新批准默认 AUTO；批量选择只增加未选项，合同重审保留已有显式确认策略。清空名单保留 ALLOWLIST 错误，恢复全部须切换模式。

助手开关表示“强制启用此 MCP”，默认 ALL；指定工具用独立窗口操作。未勾选不禁用服务器，界面说明手机端用户仍可选用。取消强制项同时解除该助手的工具子集限制，服务器权限继续生效；空强制集合与无人绑定的 enabled 服务器可正常保存、校验和发布，Snapshot 不丢弃这些服务器。选项来自服务器白名单或 ALL 的发现目录。窗口关闭、Escape 和完成保留当前草稿，生效仍需保存发布；不可用旧选择可移除，不能重新新增。旧服务器级引用由明确转换动作恢复原服务器并写 ALL。

两端名称/用途各一行截断，按两者搜索，完整定义单独查看；每页最多 50 项，有界滚动，跨页保留选择。批量动作明确针对完整目录或全部搜索结果并保留范围外选择。助手摘要最多两个标签及剩余数量。未发现的审核定义提示未核实；完整发现缺失才报删除。ALL 目录暂缺与服务器白名单越界分别提示。中英文及 Problem/Validation 文案在 `console/src/i18n/locales/mcpTools.ts`。

## Android 接线

实际源码和消费者验证由 `rikkahub_mcp` 拥有。以下路径相对其 `app/src/main/java/net/weero/measix/pilot/`。本次 Core 修改保留字段结构，直接修订未正式发布的 v5 目标，产品版本、合同号、Snapshot 版本及数据库迁移不变；已发布 v4 继续保留原引用语义。不同 v5 候选仍需固定 source/包/材料摘要，schemaVersion 相同不证明兼容。Core 新包部署保持现有 active Release，不自动重新发布。

当前 Android 源码仍把受管助手可用企业 MCP 限定为 binding：`AssistantUsagePreferences.resolveEnterpriseAssistantUsage` 只合并用户 MCP；`AssistantPreferenceMutation.requireMcpBinding` 拒绝未绑定企业服务；`AssistantMcpChoice.mcpChoices` 隐藏未绑定服务；`McpRuntimeCoordinator.withCurrent` 在受管助手缺少 binding 时拒绝执行。四处必须一起调整，不能只解除 UI 或执行层检查。本次仅交付 Core 合同、Console 和导出，Android 接线及实际消费尚未实施/验证。

| owner | 接线工作 |
| --- | --- |
| `data/enterprise/PlatformWire.kt`、`PlatformWireCodec.kt` | 从 Core OpenAPI 生成 v5 DTO，保留 v4；受支持响应忽略未知字段，已知 required/null/枚举/hash 仍验证 |
| `PlatformSnapshotMapper.kt`、企业资源/助手领域对象 | 映射只读许可及 mcpBindings，验证唯一性、enabled 引用和子集闭合；不写用户 MCP/OAuth 配置 |
| `data/configuration/AssistantUsagePreferences.kt`、`ResolvedConfiguration.kt` | 派生强制项与真实额外选择的并集，再与当前获准服务相交；v5 未绑定服务可显式选用，v4 不放宽；同服务器去重且 binding 限制优先，失效引用保留原因 |
| `data/datastore/AssistantPreferenceMutation.kt` | 复用现有 deployment/user/assistant 使用偏好；允许选用本域已下发 enabled 服务，强制项不可关；临时强制不抹掉真实偏好，渲染/点击强制项不制造偏好；旧页面差量编辑基于真实偏好和最新强制项复验；移除仅企业启用项恢复未选，保留原主动选择；重置只清除额外项 |
| `service/AssistantMcpChoice.kt`、企业 MCP/助手设置页 | 展示全部本域可用企业服务及失效引用，区分强制只读与额外可选；allowLocalMcp/allowLocalAssistants 不禁止企业助手追加企业服务；说明取消强制项解除助手限制 |
| `data/ai/mcp/McpProtocol.kt`、`McpCatalogWire.kt`、`McpCatalogStore.kt` | 保留完整原始 Tool，新增逐工具 JCS hash 校验；区分动态目录和发布许可，缓存恢复/刷新不能重写许可 |
| `McpRuntimeCoordinator.kt`、`data/ai/tools/TurnToolSetFactory.kt` | 固定本轮工具交集、定义和确认策略；未绑定额外项仍受服务器许可/hash/approval 约束，强制项叠加助手限制且不能重复装配绕过；Master/Child/Target/辅助入口按目标助手配置复验，Caller 不能扩大 Target 能力 |
| `McpServerRuntime.kt`、`service/turn/ToolBatchRunner.kt` | 在原配置/Session gate 内复验许可、目录及 generation；仅 REQUIRE_CONFIRMATION 追加逐次确认，复用原暂停/继续 owner |

运行准入、已承诺调用及未知结果沿原生命周期；停用、删除、Session 失效或撤权在原执行 gate 和版本屏障内阻断，配置失败保全 Applied、身份和历史，禁止降级解析、清库或自动重放。强制启用只影响装配和开关，不自动调用，也不保证连接/权限/目录可用，失败沿原诊断恢复。Direct 的过滤属于客户端执行合同，Relay 保持资源级准入及透明转发；Gateway 仍是独立 S0.3 目标。

共享材料在 `api/fixtures/client-integration/`：`cases.json` 验证 wire，`reference-cases.json` 验证引用，`mcp-tool-contract-vectors.json` 验证完整定义 JCS。`scripts/export-client-integration.mjs` 导出到 `api/generated/android/integration/`；修改权威源后重新导出，不手改副本。

## 验证入口与边界

- 后端：在 `backend` 运行 `go test ./internal/hub/capability ./internal/hub/httpapi ./internal/contract -count=1`；完整回归见 testing（源仓库：`measix-platform-core/docs/testing.md`）。
- Admin：`pnpm -C console test`、`typecheck`、`e2e:typecheck`、`build`。
- 浏览器：完成 production build 后运行 `node scripts/e2e-harness.mjs`，使用真实 Hub/Relay、隔离 SQLite 和确定性 MCP；`e2e/mcp-tool-governance.spec.ts` 覆盖发现/编制/确认/保存/发布、drift/失败/删除、长目录/分页和 320px；`admin-console-review.spec.ts` 覆盖 11 个管理路由及公告输入保护。
- 启动脚本：`node --test scripts/real-device-preset.test.mjs scripts/admin-build-snapshot.test.mjs`；浏览器 harness 固定服务的 SPA 副本及 hash，设备 preset 回传同 ID 发现记录，验证构建身份并保全重启数据。

运行日志、截图和构建身份留在本地 `.artifacts/`，提交和变更历史由 Git 保存。本文件不重复历史测试计数或移动候选的 PASS。Core 回归与导出不能替代 Android 原生 consumer/device、真实供应商执行或阶段 Freeze；发布流程见 release（源仓库：`measix-platform-core/docs/release.md`）。
