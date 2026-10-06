# Direct MCP 工具治理实施与 Android 对接

语义权威为同级架构仓库的 [Control Protocol §10.7.1](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md) 和 [Admin Product Requirements §9.5](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-admin-console-product-requirements.md)。本文件维护 Core 实现、操作与消费者接线；版本扩展见 [协议兼容实现](protocol-compatibility.md)。当前编制使用 Snapshot v5，已发布 v4 保留独立 DTO、原始 bytes/hash 和重新发布路径。

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

`POST /api/admin/v1/draft/mcp/{mcpServerId}:discover` 使用 `expectedDraftRevision` 和 CSRF。普通上游必须已应用当前配置；工作区来源还需选择已开通、已连接的用户。Hub 从真实 binding 和私有 Secret owner 取得连接，浏览器不能提交 endpoint 或凭据。

发现执行 initialize、notifications/initialized、分页 tools/list 和会话关闭，不执行 tools/call。支持 JSON/SSE、协商协议版本和 Session ID；SSE 中的服务端 ping 返回空结果，未声明的能力请求返回 Method not found 后继续等待目录。[MCP 传输规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) 和 [ping 规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/ping) 定义这些交换。

总超时 20 秒、响应头超时 10 秒、会话关闭最多 2 秒；目录最多 64 页、4096 工具，响应累计最多 4 MiB。完整成功才替换目录；重复名称/JSON key、循环游标、缺失 tools capability、不完整分页或非法 Tool 均拒绝。远端正文、私有 URL 和凭据不进入 Problem。

工具摘要使用完整原始 Tool JSON 的 RFC 8785 JCS UTF-8 bytes，经 SHA-256 编码为 `sha256:` 加小写十六进制。description、schema、annotations、`_meta` 和扩展均参与；对象 key 重排不改变摘要，数组顺序保留。JSON-RPC 信封及分页元数据不参与。

网络 IO 不持事务。完成后在短事务中重查草稿 revision、来源配置及认证版本，再以 CAS 更新草稿。失败或迟到结果保全原目录、许可和 revision。普通 Save 必须回传服务器的只读发现记录；新增或重审许可必须匹配该来源的发现证据，旧批准项可保留以展示 drift。Validate/Preview/Publish 共用来源、合同和引用校验；停用服务器保留结构合法的名单，重新启用时恢复审核要求。

## Admin 操作与草稿保全

有修改时“保存草稿并发现工具”先保存整个草稿，保存失败停止发现。期间新增编辑会保留并要求再次保存；保存/发现响应只更新已提交基线及服务器证据，不覆盖后续本地修改。重新加载使旧请求失效；加载期间新增编辑保留旧基线并提示冲突，防止直接覆盖其他管理员的新 revision。

服务器 ALL 默认收起只读目录；查看、搜索、发现不编制许可。只有明确切换 ALLOWLIST 后显示选择和确认设置。新批准默认 AUTO；批量选择只增加未选项，合同重审保留已有显式确认策略。清空名单保留 ALLOWLIST 错误，恢复全部须切换模式。

助手独立绑定服务器，默认 ALL；指定工具用独立窗口操作。选项来自服务器白名单或 ALL 的发现目录。窗口关闭、Escape 和完成均保留当前草稿，生效仍需保存发布；不可用旧选择可移除，不能重新新增。旧服务器级引用由明确转换动作恢复原服务器并写 ALL。

两端名称/用途各一行截断，按两者搜索，完整定义单独查看；每页最多 50 项，有界滚动，跨页保留选择。批量动作明确针对完整目录或全部搜索结果并保留范围外选择。助手摘要最多两个标签及剩余数量。未发现的审核定义提示未核实；完整发现缺失才报删除。ALL 目录暂缺与服务器白名单越界分别提示。中英文及 Problem/Validation 文案在 `console/src/i18n/locales/mcpTools.ts`。

## Android 接线

实际源码和消费者验证由 `rikkahub_mcp` 拥有。以下路径相对其 `app/src/main/java/net/weero/measix/pilot/`，实施前核对当前源码；不以过去调研 SHA 推断完成状态。

| owner | 接线工作 |
| --- | --- |
| `data/enterprise/PlatformWire.kt`、`PlatformWireCodec.kt` | 从 Core OpenAPI 生成 v5 DTO，保留 v4；受支持响应忽略未知字段，已知 required/null/枚举/hash 仍验证 |
| `PlatformSnapshotMapper.kt`、企业资源/助手领域对象 | 映射只读许可及 mcpBindings，验证唯一性、enabled 引用和子集闭合；不写用户 MCP/OAuth 配置 |
| `data/configuration/ResolvedConfiguration.kt` | 所有受管入口应用服务器上限，企业助手再应用绑定与子集；本地偏好不能扩大企业许可 |
| `data/ai/mcp/McpProtocol.kt`、`McpCatalogWire.kt`、`McpCatalogStore.kt` | 保留完整原始 Tool，新增逐工具 JCS hash 校验；区分动态目录和发布许可，缓存恢复/刷新不能重写许可 |
| `McpRuntimeCoordinator.kt`、`data/ai/tools/TurnToolSetFactory.kt` | 固定本轮工具交集、定义和确认策略；覆盖 Master/Child/Target/辅助入口，报告不可用原因 |
| `McpServerRuntime.kt`、`service/turn/ToolBatchRunner.kt` | 在原配置/Session gate 内复验许可、目录及 generation；仅 REQUIRE_CONFIRMATION 追加逐次确认，复用原暂停/继续 owner |
| 企业 MCP/助手设置页 | 只读展示批准/可用范围和失配原因；不得以静默空目录掩盖错误 |

运行准入、已承诺调用及未知结果沿原生命周期；配置失败保全 Applied、身份和历史，禁止降级解析、清库或自动重放。Direct 的过滤属于客户端执行合同，Relay 保持资源级准入及透明转发；Gateway 仍是独立 S0.3 目标。

共享材料在 `api/fixtures/client-integration/`：`cases.json` 验证 wire，`reference-cases.json` 验证引用，`mcp-tool-contract-vectors.json` 验证完整定义 JCS。`scripts/export-client-integration.mjs` 导出到 `api/generated/android/integration/`；修改权威源后重新导出，不手改副本。

## 验证入口与边界

- 后端：在 `backend` 运行 `go test ./internal/hub/capability ./internal/hub/httpapi ./internal/contract -count=1`；完整回归见 [testing](testing.md)。
- Admin：`pnpm -C console test`、`typecheck`、`e2e:typecheck`、`build`。
- 浏览器：完成 production build 后运行 `node scripts/e2e-harness.mjs`，使用真实 Hub/Relay、隔离 SQLite 和确定性 MCP；`e2e/mcp-tool-governance.spec.ts` 覆盖发现/编制/确认/保存/发布、drift/失败/删除、长目录/分页和 320px；`admin-console-review.spec.ts` 覆盖 11 个管理路由及公告输入保护。
- 启动脚本：`node --test scripts/real-device-preset.test.mjs scripts/admin-build-snapshot.test.mjs`；浏览器 harness 固定服务的 SPA 副本及 hash，设备 preset 回传同 ID 发现记录，验证构建身份并保全重启数据。

运行日志、截图和构建身份留在本地 `.artifacts/`，提交和变更历史由 Git 保存。本文件不重复历史测试计数或移动候选的 PASS。Core 回归与导出不能替代 Android 原生 consumer/device、真实供应商执行或阶段 Freeze；发布流程见 [release](release.md)。
