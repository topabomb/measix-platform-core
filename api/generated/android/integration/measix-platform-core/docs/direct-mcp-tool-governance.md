# Direct MCP 工具编制、助手绑定与 Android 对接

> 实施日期：2026-10-06。范围：未稳定 Snapshot v5 的 Core/Admin/Portal 增量；保全已发布 v4。Android 源码与原生消费者验证由 Android 仓库承担，本轮 Core 验证不代替原生验收。语义权威为同级 Architecture 的 Control Protocol §10.7、§10.14 与 Admin Product Requirements §9.5、§9.7；本文件维护具体实现、交互落实、消费者接线和验证证据。

## 1. 目标与验收边界

管理员能从真实 Direct MCP 发现工具，审阅完整定义，选择企业允许暴露的工具及调用确认策略，再为每个企业助手选择子集。保存、刷新、审核、Preview 与 Publish 沿现有 Managed Draft 工作流完成。配置非空白名单时限制工具；未编排白名单则动态使用全部工具。服务器有白名单时合同变化必须重新审核发布。Relay 保持资源准入和透明转发，不解析 MCP 业务或承诺逐工具服务端 ACL。

本次交付包括 Portal 的 Client API 传递类型/schema/来源摘要、Android 交接和真实 STANDARD/CUSTOM 浏览器回归（Portal 不增加 Admin API 或原生执行权限），以及架构同步、Admin/Client OpenAPI、canonical fixtures、Go/TypeScript 生成产物、Core 工具发现/校验/编译/持久化、Admin 编辑与审查、Android 导出材料和定向/全量/真实浏览器验证。Android APK、设备执行、Gateway daemon 与 S0 阶段验收不属于本次完成声明。

## 2. 实现数据与写入归属

### 下发类别与启用方式

| 类别 | 当前实现/阶段 | 呈现、发现与授权 |
| --- | --- | --- |
| Direct Managed MCP，普通上游 | 当前 Hub/Relay 实现 | 企业资源只读、自动启用；Admin 用生效私有 binding 发现，客户端用 Relay 发现并校验发布许可；模型使用已绑定服务器，工具白名单非空才过滤 |
| Direct Managed MCP，用户工作区来源 | 当前 workspace 实现 | 同一企业资源的远端目录按用户会话发现；Admin 选已连接用户取审核样本，Android 用实际登录用户目录取交集。样本身份不成为共享 Snapshot 里的授权身份 |
| 用户自己的 MCP | Android 现有本地能力 | 由 `allowLocalMcp` 控制配置准入，沿用户原启用/授权流程；不合并企业许可，也不覆盖企业资源 |
| Enterprise Tool Gateway | S0.3 目标，当前 Core 尚无 Gateway daemon | 后续独立 discover/invoke 工具对与服务器强制授权；不与 Direct 互相替代。不得以本次 Direct 实现宣称 Gateway 完成 |

白名单适合助手作为可审查的企业能力包：远端新增时权限保持不变，预览可以准确列出工具，差异可追溯。黑名单默认放行新增工具，特别容易把后加写入/外发工具交给既有助手，并且同名 schema/说明变更仍需额外机制；因此本轮不提供黑名单模式。需要限制时选择白名单；不限制时明确选 ALL 并动态发现，不提供黑名单。大量工具的按需检索与服务端强制授权仍是 Gateway 后续方向。

### 助手如何编排

本轮的工具编排是“绑定具体能力 + 助手指令 + 原生工具循环”。例如检索助手只绑定 `search_records`、`fetch_record`，指令规定先检索、再按 ID 读取、最后总结；资源上的确认策略决定原生循环是否暂停向用户确认。Core 不执行助手、不保证指令顺序、不新增工作流状态。若后续要求确定的依赖图、人工节点、幂等执行与补偿，应独立定义 Workflow 合同和执行 owner；Gateway 则负责工具发现/引用/授权，二者不能混作本次白名单功能。

当前采用可选服务器和助手白名单，不提供 wildcard 或 ALL_EXCEPT 编制语法；其未来扩展须先定义实际场景和新的合同含义。资源 `toolAccessMode=ALL, allowedTools=[]` 表示不限制，动态发现全部工具，不增加调用确认或企业合同锁定；ALLOWLIST 必须非空，按名称、合同 hash 与确认策略执行。助手已绑定服务器的 `toolSelection=ALL, toolNames=[]` 表示未编排，不额外限制；ALLOWLIST 必须非空，按名称过滤并不得越过服务器白名单。删除服务器 binding 表示该助手不用此 MCP，`enabled=false` 表示服务器停用。工具身份是 MCP 资源 ID 加大小写敏感原始 name；重命名按删除/新增处理。

Admin 草稿资源增加可缺省的 `toolAccessMode` / `allowedTools` 和只读 `toolDiscovery`。缺省表示存量未编制；新建明确写 ALL 和空名单，默认不限制。每项允许工具包含 name、contractHash、approvalPolicy，以及服务器审核保存的完整 definition。调用确认枚举为 AUTO / REQUIRE_CONFIRMATION；新选择默认 AUTO（无需额外确认），管理员可明确选择 REQUIRE_CONFIRMATION（每次调用前确认）；合同重审、全选和保存发布保留已有显式策略。默认值只属于新建批准动作，不用于为缺失 wire 策略补值。工具发现包含来源指纹、发现时间及完整候选定义。上述字段归唯一 Managed Draft owner；发现完成使用 expectedDraftRevision 比较并交换，网络 IO 不持有数据库事务或配置锁。普通 Save 不允许伪造发现记录或未经发现的批准定义。不限制模式发布无需先取得目录；发现目录只用于显示与编制可选限制。

Client v5 MCP 保留原标准资源字段并投影 toolAccessMode、允许项的 name/contractHash/approvalPolicy；不下发发现记录、完整定义、来源指纹、上游 URL 或 Secret。助手由 mcpServerIds 升级为 mcpBindings，每项为 mcpServerId、toolSelection 与明确 toolNames。Admin 保留只读 legacy 引用恢复材料，帮助存量草稿显式确认模式，不用读取动作推断全部授权。

服务器 ALLOWLIST 的逐工具 contractHash 使用完整原始 MCP Tool JSON 的 RFC 8785 JCS UTF-8 bytes 的 SHA-256，编码 sha256:小写十六进制。只排除 JSON-RPC envelope 与分页/transport 元数据；description、schema、annotations、_meta 和扩展内容均纳入，避免未审核合同替换批准项。对象 key 重排不改变摘要，数组顺序和正文原文保留。客户端不能用普通 JSON 序列化 hash 替代 JCS；ALL 使用当前真实目录，不伪称逐项审核。

v4 使用独立原始 McpDefinition/Assistant wire DTO；下载、重发布及 canonical hash 保持旧字段和语义。草稿缺失新字段是未编制，保存不会自动赋权，Validate/Preview/Publish 要求完整 v5。无 MCP 的草稿及助手可用空集合。存量 JSON 数据由读取适配保留旧引用，不清库、不改不可变 Release，不新增无必要的 SQL 表。

### 管理员的操作模型与协议状态

| 助手设置 | v5 编译结果 | 当前及未来工具 |
| --- | --- | --- |
| 不使用此 MCP | 无该服务器 binding | 不提供此服务器工具 |
| 使用此 MCP → 全部工具（默认） | toolSelection=ALL, toolNames=[] | 动态使用该服务器可用工具；服务器权限仍生效 |
| 使用此 MCP → 指定工具 | toolSelection=ALLOWLIST, toolNames=[...] | 仅所选名称，新增不自动加入 |
| 指定工具但删完选择 | ALLOWLIST + []，校验错误 | 阻止发布，不能悄悄扩大为全部工具 |

服务器资源的“企业工具权限”独立于助手编排：ALL 使用动态目录，不增加调用确认；ALLOWLIST 按审核合同限制工具范围，是所有入口共同上限。工具范围、合同锁定和调用确认分别表达，助手选择不改变服务器确认策略。界面避免把两者都叫编排。服务器停用用 enabled，助手停用用绑定开关，不复用空名单。两种 ALL 均是显式 wire 枚举；缺失/null/未知枚举不补默认、不进入运行。

## 3. Core 工具发现与发布

Admin 使用 `POST /api/admin/v1/draft/mcp/{mcpServerId}:discover`，携带 CSRF、expectedDraftRevision，工作区来源可携带发现所用 userId。要求先保存当前编辑，且使用该草稿资源实际绑定的已生效 upstream/config/Secret。工作区 MCP 使用已开通用户的不可变有效 binding；发现身份只用于取得目录，不写入共享 Snapshot 或形成用户授权。

发现客户端仅 initialize、notifications/initialized、分页 tools/list 和必要的会话关闭，不执行 tools/call。支持 JSON 与 SSE JSON-RPC 响应、Mcp-Session-Id 和协商协议版本；有界超时、最多 64 页/4096 工具/4 MiB 响应，拒绝重复名称、游标循环、无 tools capability、无效 Tool 或不完整分页。完整成功后才替换候选；失败不清已批准内容、不推进草稿 revision。禁止把远端原始异常正文、凭据或私有 URL 放进日志/Problem。

来源指纹绑定资源路由和具体上游/工作区配置及认证版本。来源发生变化后旧发现不得授权新目标。迟到结果先复验 revision 与来源；冲突返回明确 Problem，客户端保留编辑并显示刷新动作。网络错误、协议错误及超限使用不同稳定 code 和可行动说明。

Save 允许保留旧批准项用于展示 drift；新增或重新批准的 name/hash/definition 必须来自服务器记录的发现候选，不能靠浏览器计算 hash。Validate 对缺少编制、来源失配、被删除/变化的批准工具、重复项、无效确认策略和助手悬空/越界引用产生具体路径。Preview/Publish 都使用同一校验及 canonical projection，排序工具和绑定集合，不排序 JSON 内部数组。刷新不会改变 active Release、Runtime 或 managedGeneration。

停用服务器可保留结构合法的审核名单，不要求连接不可用来源来重新发现；助手不能绑定已停用服务器。重新启用 ALLOWLIST 时恢复来源及合同核验要求。

Release 内容保存审核定义便于审查和追溯，Client Snapshot 不含定义。变更列表计数只基于发布内容，不把发现时间或未选择候选变化当作已发布 MCP 改动。现有 runtime 资源级额度、身份撤销、generation barrier、私有凭据注入和工作区用户隔离继续由既有 owner 执行。

## 4. Admin 界面需求的具体落实

### MCP 资源编辑

- 连接配置后提供“发现工具”；存在未保存修改时动作明确为“保存并发现工具”，保存失败保留输入并停止发现。
- 发现期间禁用重复命令，保留已发现目录，显示进度；失败显示简短原因和可复制诊断，不把空列表伪装为成功。
- 工具区显示搜索、候选/已允许数量、名称/描述摘要、选择和确认策略。显示“当前不限制，动态发现全部工具”或白名单数量；只有明确切换为指定工具后才能勾选工具，批量选择编制当前目录或搜索结果的名单，主动切换“全部工具”恢复不限制；指定工具模式清空后阻止发布。详细 schema/annotations 在独立详情窗口查看，移动端避免宽 JSON 常驻和横向溢出。
- 未发现、未编制、空目录、来源已改变、同名合同已变、已批准工具被删除分别提示。重新批准必须是显式动作，不能把刷新当作审核；非空服务器白名单缩减对越界助手形成可见错误；切换服务器为全部工具恢复不限制，不删助手绑定。
- 企业允许工具是草稿事实；来源凭据不进入浏览器持久化。
- 当前目录与保留的批准项统一搜索，每页最多 50 项，目录有界滚动；名称及用途各一行截断，完整定义按需查看。翻页回到工具列表起点，搜索/刷新复位页码；工具选择跨页保留。ALL 默认收起目录，显式发现成功可展开；只读查看没有选择、清空及确认策略动作，不改变工具范围。指定工具的批量操作明确针对完整目录或搜索结果，保留范围外选择与确认策略，不能重审已变更定义。尚未发现当前目录不等于工具被删除：保留定义并提示未核实；完整发现确认缺失才显示删除错误。ALL 模式发现是可选的，不把“没有目录”当作发布前置条件。保存并发现明确保存整个草稿且不会发布。

### 助手模型与工具编辑

- 按已启用 MCP 资源显示工具 picker，服务器非空白名单存在时只能选择已允许工具，否则选择当前发现工具；每个服务器独立启用，默认“全部工具”不额外限制；“指定工具”必须至少选一项；选项来自服务器白名单或其当前发现目录。
- 明确提示“选择工具决定可用能力，步骤与依赖写入助手指令”。通过独立选择窗口显示服务器名、已选/可用数量、复选框及已选/未选状态，支持仅看已选和明确范围的批量动作；完成或关闭保留当前草稿。支持切换全部工具（恢复不限制）、移除服务器绑定（停用），并保留失效项供移除或重新批准。
- 存量服务器级引用显示转换提示，明确动作保留原服务器并转换为 ALL 和空工具名单，保持不限制语义。
- 同一资源移除、禁用或工具取消批准后，引用在原区域标红，Validation 链接定位到工具区。
- 对 ALL 服务器，助手已选名称暂不在当前目录时显示“暂不可用”提示并保留选择；这不同于超出服务器 ALLOWLIST 的权限错误。显示选择数量，已禁用服务器仍尽可能显示名称供修复。
- 指定工具通过独立选择窗口操作，支持名称及用途描述的不区分大小写搜索，名称和描述各一行截断，完整定义单独查看；过滤不移除已选项。批量动作无查询时选择整个可用目录，有查询时明确选择搜索结果并保留范围外选项。已选项最多展示两个紧凑标签，其余显示数量，完整选择仍保存在同一工具名单中。

### 审查与响应式体验

- Preview 展示实际下发的逐工具名单、hash 诊断、确认策略及助手选择；不把 server-only 发现记录当作 Client 内容。
- 桌面复用现有工作台，手机使用同一模型与纵向卡片。详情动作可达，长 description/schema 在 body 内滚动；关闭详情保留编辑。
- 真实浏览器验收包括发现、选择、确认策略、助手子集、保存重开、Preview、Publish、远端 drift、失效引用修复、发现失败保全和 320px 窄屏。

## 5. Android 下一阶段接线要求与建议

以下 owner 基于本轮只读调研的 Android `80cd9b2bb004e8fed53c2d346f98eacf58f4c4a9`；本次未改其文件。路径以 `app/src/main/java/net/weero/measix/pilot/` 为前缀：

| 接线位置 | 必须调整的行为 |
| --- | --- |
| `data/enterprise/PlatformWire.kt`、`PlatformWireCodec.kt` | 从新 Core OpenAPI 重新生成，保留 v4 独立 DTO；v5 必填许可模式/绑定模式、closed shape、非法枚举/hash/null 拒绝，禁止沿用 server-only 引用 |
| `data/enterprise/PlatformSnapshotMapper.kt`、企业领域资源与 Assistant 映射 | 当前仍检查 assistant.mcpServerIds；升级为显式 ALL/ALLOWLIST 和工具子集闭合校验，并保存只读 name/hash/approvalPolicy |
| `data/configuration/ResolvedConfiguration.kt` 的 ConfigurationResolver | 企业许可来源保持领域隔离；全部受管服务器入口施加资源上限，企业助手再施加子集，不让本地工具 policy 覆盖企业要求 |
| `data/ai/mcp/McpProtocol.kt`、`McpCatalogWire.kt`、`McpCatalogStore.kt` | 保留原始完整 Tool；当前目录 digest 不是逐工具 JCS 许可摘要，需要新增独立摘要校验。刷新、新增、失配、持久缓存恢复均不得改写发布许可 |
| `data/ai/mcp/McpRuntimeCoordinator.kt`、TurnMcpCapabilitySnapshot | 捕获 name/contractHash/确认策略和不可用原因；Master、Child、Target 与辅助入口统一装配，不能只过滤设置页或聊天主入口 |
| `data/ai/tools/TurnToolSetFactory.kt`、`data/ai/mcp/McpServerRuntime.kt` | 向模型只送匹配交集；admitInvocation 复验同一授权、目录、Session/config generation 和调用确认，避免撤销与目录变更竞态 |
| `service/turn/ToolBatchRunner.kt` 与企业 MCP/助手设置页 | 复用既有确认交互和暂停/恢复 owner；企业许可只读，解释已批准但当前失配的工具，不用静默空列表掩盖原因 |

建议分三步接线并分别验证：先 DTO/mapper 与 v4/v5 fixtures，再目录及调用门控和既有确认循环，最后企业设置/助手详情与实际设备流。共享资料含 `cases.json` 的缺失/null/旧字段/非法 grant 反例、`reference-cases.json` 的越界/重复引用，以及 `mcp-tool-contract-vectors.json` 的完整定义 JCS 摘要向量。先使用这些用例形成 Android Red，再实施 Green；Core 参考 oracle 不能替代消费者测试。

从 Core 导出重新生成 PlatformWire；不得手改 generated 类型。保留 v4 DTO/decoder/cases；v5 对缺失/null toolAccessMode、allowedTools、mcpBindings、toolSelection、toolNames，未知枚举、重复名称、非法 hash、悬空及越界引用严格拒绝。普通下载错误保留旧 Applied 与企业身份，提示同步/升级，不降级解析、不清库。

PlatformSnapshotMapper 将资源许可与助手 binding 映射到企业只读领域对象，不写用户 McpServerConfig/OAuth。在 ConfigurationResolver 和所有受管 MCP 入口按 toolAccessMode 应用服务器权限；企业助手按 toolSelection 应用可选子集，不能通过额外选择同服务器绕开。用户自有 MCP 仍按 allowLocalMcp 和原本地工具 policy 管理。

McpCatalogDiscovery 保留完整 Tool；McpCatalogStore 区分远端已确认目录和企业发布许可。服务器 ALLOWLIST 时逐工具校验完整 JCS hash；服务器非空白名单时忽略新增，变更/删除的批准工具不可用；服务器无白名单则动态采用当前目录，助手非空名单再按名称过滤。服务器 ALL 不增加调用确认；服务器 ALLOWLIST 按显式 approvalPolicy 执行，AUTO 不追加企业确认，REQUIRE_CONFIRMATION 才逐次暂停等待用户允许。二者均保持原有运行准入和系统权限。目录刷新不得更新发布许可；本轮已发送 definitions 固定，检测失配后拒绝尚未承诺的新调用。已承诺调用的成功结果和未知结果处理沿原生命周期，禁止自动重放。

TurnMcpCapabilitySnapshot 捕获经模式过滤后的当前工具；服务器 ALLOWLIST 要求 hash 匹配，ALL 使用真实动态定义且不增加调用确认，以及明确不可用结果；TurnToolSetFactory 接入企业 approvalPolicy。McpServerRuntime.admitInvocation 在原配置 writer/Session gate 内复验 name/hash/助手授权和确认要求，所有入口一致。复用已有 ToolBatchRunner 确认暂停/继续协议，不创建第二审批状态机。

设置页只读展示企业来源、强制启用、批准/可用工具及失配原因；助手页只读展示已绑定服务器和全部/指定工具模式。自动启用不自动调用；Starter 不 auto-send。Gateway 继续独立装配 discover/invoke 原子对，不能按 Direct 工具名单过滤它或建立互相 fallback。

建议 Android 定向覆盖：v4/v5 解码隔离、空与缺失区别、跨服务器同名、非空白名单新增不暴露、ALL 动态新增可用、未绑定停用、同名 description/schema/_meta drift、合同匹配缓存恢复、辅助入口与子运行不绕过许可、审批暂停/继续、调用前撤销竞态、428 不重放、配置同步失败保全。设备覆盖首次发现、离线、通知刷新和真实 UI；Core 导出/浏览器通过不替代这些消费者证据。

## 6. 后续方向与当前限制

Direct 适合少量稳定工具，模型直接看到当前可用 schema；有服务器白名单时要求审核合同匹配；Gateway 适合大量目录，模型按需 discover 后取得 toolRef，由 Gateway 强制执行 schema/authorization。共用 JCS 和审核表达，但不提前引入 Gateway ID、搜索、toolRef 或 workflow。Gateway S0.3 仍 READ_ONLY；Direct 确认策略不自动扩张其阶段范围。

本轮白名单是企业客户端行为合同；Relay 不核对 assistant/tool name，服务器端逐工具强制授权需要上游权限或 Gateway。合同摘要不能证明远端实现语义不变或真实只读。若将来要求完整助手不可扩展工具集合，必须额外定义 Local/Search/Skill/子助手等准入，不能用 Direct 名单推断。

“不提供”必须区分：未绑定 MCP 是该助手不使用这台服务器；ALLOWLIST 名单外工具不进入模型，也不能被原生调用入口绕过；当前目录缺失/不可达是暂不可用并显示原因；非法或不支持协议是配置接收失败，保全旧 Applied 并提示同步/更新。它们都不等价于不限制。ALL 是管理员主动选择的动态能力范围，不承诺可审查的固定工具集合。

当前不提供黑名单，是因为本轮限制目标是明确工具集合且无独立排除策略需求；不提供 workflow DSL，是因为 Core 当前不执行助手流程。它们是范围与 owner 的说明，不是架构永久禁止。后续可在新协议中增加选择策略/独立流程定义，先明确管理体验、动态变更与授权含义，再按合同升级；不修改已有枚举的默认语义来偷渡扩展。Gateway 的服务端授权/按需目录与 Direct 的客户端工具筛选仍有实际差异。

## 7. 验证计划与证据

按 Red→Green：先添加 v5 closed schema/v4 保全、目录摘要/分页/失败/来源竞态、授权闭合与 canonical projection 测试；记录失败后实现。生成所有消费者材料，并执行合同、Hub/Relay、Admin typecheck/unit/build、工具脚本回归、真实 production Admin E2E 与手工浏览器交互。新增 wire 更新 protocol-baseline，记录源码/合同/产物摘要，不称为新的阶段 Freeze。

2026-10-06 完成 Core/Admin/Portal 实施、全页 Admin 审查与 Git 变更复核。本次验证基线为 Core `59b2f4157e2fc8032e91a18bed0c1ae71afdcf9c` / Architecture `719267f65dca487913983b09075e32367c8eb554` / Portal `1ec94b1a7d5a45eb588d2085b91567dded3c4ed0` 上的候选内容，提交记录见各仓库 Git 历史；不是基线干净 commit 的发布验收，不构成新 S0 Freeze。实际产物、源码/合同/构建摘要和验证日志见 `.artifacts/mcp-tool-governance-verification.json`；全页审查范围、修复和复核方式见 [Admin 审查记录](admin-console-review-2026-10-06.md)。

| 已执行验证 | 结果与范围 |
| --- | --- |
| 新协议/目录/保存校验 Red→Green | 先观察 v5 allowedTools 缺失、发现接口未实现导致失败，再通过合同、完整定义摘要、分页/SSE/session、拒绝不完整/超限目录、授权闭合、伪造目录、保存失败保全、迟到发现 CAS 和工作区发现期间撤销用例 |
| ALL/ALLOWLIST 与停用校验 Red→Green | 先观察新模式 unknown-field、助手绑定开关缺失、停用仍要求发现的失败；随后通过显式模式投影、动态服务器可限制名称、白名单为空不扩大权限、非法模式和停用保留审核名单测试 |
| `go test ./... -count=1`、`go vet ./...` | 全量通过；覆盖 Hub/Relay、v4 保全、发布/重发布、工作区与生成合同。追加的发现 HTTP 定向测试验证 Admin、CSRF、closed request、revision 和无来源错误不改变草稿 |
| `go test -tags=smoke ./test/system/scenarios/ -count=1 -timeout 5m` | 通过，真实组件 smoke 回归 |
| Admin typecheck / Vitest / production build / E2E typecheck | 42 文件、259 用例通过，生产构建和两类类型检查通过 |
| Portal generate / typecheck / Vitest / production build | 新 Client 类型/schema/来源摘要同步，13 文件、95 用例通过，类型检查与生产构建通过 |
| Portal `pnpm e2e:real` / `pnpm e2e:real:custom` | STANDARD 与 CUSTOM 各 2 个真实浏览器用例通过，Session/Feed/Usage/静态代理及手机/桌面可读性回归；不代表 Android 真机 |
| `node --test scripts/*.test.mjs`、`node scripts/checks.mjs fmt` | 59 用例和格式检查通过；格式检查按内容忽略合法 Windows CRLF 差异，不改原文件换行，仍拒绝实际 Go 格式漂移 |
| 两轮当前源生成比对 | 88 个 wire/fixture/导出产物一致（含本次审查文档导出）；标准在原位置的 Ent 生成因 Windows user-mapped section 写入失败，隔离同模块/schema 运行相同 pinned Ent CLI 后比对 291 文件一致，原 Ent 工作树未改变。该等价复核不伪称标准 `generate` 命令通过 |
| `node scripts/e2e-harness.mjs --keep` | 真实 production Admin + Hub/Relay + SQLite + 确定性 MCP；8 浏览器用例、0 skipped/flaky/unexpected；发现/选择/助手子集/保存重开/Preview/Publish、drift/删除/失败恢复、64 工具搜索/分页、全部 11 个管理页面的 1280/320px 回归、公告空内容保护、五类 Runtime、计量汇总与拓扑检查均通过 |
| 手工真实 Admin 交互 | 隔离环境用 CUA 操作中文版发现、搜索清除、详情、实际预览、失败恢复，以及助手绑定关闭/开启默认 ALL、白名单清空仍限制、明确切换 ALL、保存和重开。截图 `.artifacts/mcp-admin-manual.png`、`.artifacts/mcp-admin-selection-manual.png`；自动窄屏截图 `.artifacts/mcp-tools-mobile.png` |

实际发现并修复：长 MCP 编辑区切换 Policy 后滚动位置仍留在底部，表单首项被固定 header 遮挡；切换分区现在复位滚动。工具搜索清除按钮产生 null，引起列表计算异常；新增最小失败测试后按空查询处理。停用服务器仍要求发现造成无法保留白名单发布停用状态，已按新测试修正。单元和真实浏览器均通过；新 E2E 的 Quasar input、首屏等待和预览标签定位按实际 DOM 修正，没有增加重试来掩盖失败。

追加 Admin 全页审查修复了目录无界渲染、服务器空白名单指向助手开关的错误提示、ALL 发现前置条件暗示、未发现误报删除、助手暂时缺失与权限越界混淆、空白公告可提交，以及全局设置的原始枚举文案。相关组件最小失败测试均已观察 Red 后通过 Green。额外 `go test -race` 因本机 CGO 未启用而未执行；不能将普通 Go 回归称作 race 检查通过。

初次工具治理验收时，Portal 已同步并验证，Android 仓库尚未修改；没有运行或宣称新版 Android consumer/device 验证。该次跨仓库 Preview 合同比对仍 FAIL，剩余为 Android 新 DTO/fixtures/manifest 未接入。Android 后续源码和测试材料更新本身不等于消费者验证完成；其完成状态以 Android 仓库的实际验证为准。Core/Portal 生成、测试和浏览器通过不能使该跨仓库 gate 转为 PASS。既有 preview.22 包和不可变 v4 Release 不受此次工作树修改影响。

## 8. 2026-10-06 调用确认纠偏

按用户明确要求撤回 ALL 隐含逐次确认，工具范围、审核合同锁定、调用确认分别定义。当前 v5 wire 结构和枚举保持不变；服务器 ALL 动态发现、不追加企业确认、不锁定审核合同，助手 ALL 不增加过滤，ALLOWLIST 的 AUTO/REQUIRE_CONFIRMATION 显式生效。身份、资源、generation、额度和系统权限沿原运行准入；Relay 继续透明转发。

Admin 新选工具及选择当前全部的新增项默认 AUTO，中文为“无需额外确认”。已有显式策略在全选、重新审核合同、保存重开、Preview/Publish 中保留。重新审核曾重置策略的问题已通过最小测试定位并修复；明确取消选择或切换服务器 ALL 会按该动作移除对应批准项，不在其他动作中重写策略。Android 接线只在 REQUIRE_CONFIRMATION 时复用原确认循环；ALL 和 AUTO 不追加企业确认，助手筛选不修改策略。

本轮最小 Red 为四个失败断言：单选默认、全选默认、AUTO 合同重审保留、ALL 提示；定向 Green 为两组件 12 用例。日志 `.artifacts/mcp-approval-red.log` / `mcp-approval-green.log`。手工中文版真实 Admin 已验证默认 AUTO、显式确认保存重开、ALL 的实际 Preview、切回 ALLOWLIST 的空名单保护及 320px 无横向溢出；截图 `.artifacts/mcp-approval-manual-default.png`、`mcp-approval-manual-explicit.png`、`mcp-approval-manual-all.png`、`mcp-approval-manual-mobile.png`。完整自动验证结果与来源/产物复核记录在本节后续验收记录中。

本轮当前候选验证：Go 全量、vet、组件 smoke 通过；Admin 42 文件 260 用例、typecheck、E2E typecheck、production build 通过；Portal 13 文件 95 用例、typecheck、production build 和 STANDARD/CUSTOM 各 2 个真实浏览器用例通过；Node 工具脚本 59 用例、格式检查通过。完整生产 Admin+Hub/Relay 浏览器链路 8 用例通过，0 skipped/flaky/unexpected，覆盖显式确认与 AUTO 并存时的重审、全选新增项、保存重开、Preview/Publish，以及 ALL 动态目录和全页窄屏回归。

本轮没有修改 Ent、依赖或 Relay 生产代码，不将历史 Ent/race 结果作为本轮新检查。只重新生成当前受影响的 wire/fixture/Android 导出，并逐项比对两轮产物；结果、源文件/产物摘要及 Git 提交收据见 `.artifacts/mcp-approval-verification.json`。Android 消费者合同比对另存 `mcp-approval-cross-contract.log`，材料差异必须在 Android 仓库同步后独立验证，本轮不声明原生消费通过或新的 S0 Freeze。

## 9. 2026-10-06 工具目录与助手选择交互修订

本节对应用户对长工具目录、缺少用途说明、下拉位置错乱及选择状态不清楚的反馈。架构的 Admin Product Requirements / Testing Spec 已同步；只调整 Admin 编制体验及浏览器验证基础设施，不改变 v5 wire、ALL/ALLOWLIST、调用确认和 Relay 边界。Android 源码未改，Portal 没有该 Admin 编制页面，无需为本次布局变更改变其 Client 合同。

| 原交互问题 | 当前实现 |
| --- | --- |
| 全部工具下仍显示全选，点击会隐式改成白名单 | ALL 默认收起只读目录，不提供选择/清空/逐项确认；切换“指定工具”后才编制名单 |
| 大段说明撑高每张卡片，27 项仍分两页 | 名称及用途各一行截断，共用紧凑行；目录有界滚动，每页最多 50 项，27 项无需分页，完整定义单独查看 |
| 助手选择只显示工具名，难以判断用途 | 名称和用途可见并支持两者搜索，明确展示服务器批准定义及独立确认策略，查看详情不改变选择 |
| 浮动下拉在输入框上方遮住其他字段，已选与未选只靠颜色 | “选择工具”独立窗口显示 MCP 名、已选/可用数；复选框、文字状态和选中背景一致，可“仅看已选”；桌面/320px 均在视口内 |
| 选择大量工具撑高输入区域，批量动作范围不清楚 | 主编辑区最多两个标签和剩余数量；窗口跨页保留选择。无查询选择当前目录全部，有查询选择全部匹配结果，保留范围外选择及确认策略 |
| 旧选择当前不可用，或关闭选择窗口后结果不明确 | 不可用名称保留为可移除的已选项，不允许重新新增。完成、关闭按钮和 Escape 均保留当前草稿；保存并发布后才实际生效 |

保存并发现明确提示“保存整个配置草稿，不会发布”。清空指定名单仍保留 ALLOWLIST 和原错误，恢复动态全部必须主动切换模式。目录变化、旧审核定义、失效引用与确认策略保留原规则；批量选择不重新审核已经变化的合同。搜索清除的 null/纯空格均按空查询处理；按当前说明匹配到的变更工具可正常取消原批准项。

本轮逐步观察了 ALL 只读、用途搜索、查询内批量操作、变更说明清空、纯空格查询、独立窗口入口、50 项分页和“仅看已选”清空文案的最小 Red，随后实施 Green。浏览器还实际发现旧浮动菜单宽度超过视口，最终已移除该下拉实现，改为独立窗口；不能把早期下拉截图当作当前验收。浏览器过程中共享 `dist/spa` 重建曾导致页面 404，验证 harness 现将已完成的生产构建复制至独立运行目录并校验源/副本摘要一致，再从该固定副本服务；不是通过重试掩盖产品失败。

当前候选基于 Core `4012b68e46036efde54c802d46dab566032a4036` / Architecture `2f4112a5aeaa9cd3e501f37a984588337592a1c6` 的工作树，不将这些基线 commit 或历史 §7/§8 当作新候选通过证据。源码、构建、报告、截图及最终提交收据记录在 `.artifacts/mcp-catalog-ux-verification.json`。

| 当前候选检查 | 结果 |
| --- | --- |
| Admin 全量 Vitest | 42 文件、265 用例通过，含两 MCP 组件 17 用例 |
| Admin typecheck、E2E typecheck、production build | 通过 |
| Node 工具脚本 | 65 用例通过，含生产构建副本的原始字节保全、缺失入口与重复目录拒绝 |
| 格式检查 | 通过 |
| Go 合同回归及消费者资料导出 | `go test ./internal/contract -count=1` 通过；重新导出当前交接文档，逐文件与权威源及重复导出比对 |

人工使用真实 production Admin + Hub/Relay + 隔离 SQLite 操作中文版，目录为确定性测试服务的 27 项长说明及超长名称，不冒充真实第三方供应商。已验证 ALL 查看不产生编辑，名称/用途搜索、混合已选状态、仅看已选、搜索结果选择/取消保留范围外两项、嵌套详情关闭后状态不变，以及完成后保存并重新加载的持久化。桌面窗口在 1280×720 视口内，320px 无横向溢出且“完成”可达。人工截图 `.artifacts/mcp-catalog-ux-manual-dialog-desktop.png` / `mcp-catalog-ux-manual-dialog-mobile.png`；自动回归截图为 `mcp-catalog-ux-picker-desktop.png` / `mcp-catalog-ux-picker-mobile.png`。

最终 `node scripts/e2e-harness.mjs --keep` 通过：8 个浏览器用例，0 skipped/flaky/unexpected，包含独立窗口的关闭/Escape 保留选择、发现/策略/保存重开/Preview/Publish、失败/drift/删除修复、27 项长说明与 64 项分页、五类 Runtime 和用量汇总，以及 11 个管理路由的 1280/320px 回归。实际服务的生产 SPA 摘要为 `sha256:83b2d23af4f5037f7a41ec0eea218bc164cf02814cfaac691d5153cae2329035`，与本轮完成构建的摘要一致；`e2e-admin-build.json` 固定服务目录及摘要，最终报告与 meta 同为本轮生成。导出及合同检查结果一并记入验证收据。

本次没有修改后端生产源码、OpenAPI、SQL 或依赖，没有执行新的 Android consumer/device 验证，不声明 S0 阶段完成。
