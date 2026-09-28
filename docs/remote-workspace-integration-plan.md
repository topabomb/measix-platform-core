# 远程工作区与 Agent Space 集成实施方案

状态：**待实施方案**。本文记录已确认的产品目标、具体选择、实现依赖和验收要求，不代表接口、数据库或跨项目集成已经交付。

调研及复核日期：2026-09-28。本文属于 Core 的实施规划；跨组件语义变更必须先落入架构权威，再按 OpenAPI → fixtures → generated artifacts → tests → implementation 实施。本文不替代现有架构合同，也不修改历史发布内容。文中的新增字段、路由及状态语义均为实施要求，不能按现有接口直接调用。

## 1. 目标、范围与确定选择

Core 将“远程工作区”作为可选企业能力。Admin 可以配置并热更新 Agent Space 集成，为指定企业用户创建、断开、恢复或删除远程工作区，并管理其中的文件。开通用户通过现有企业托管 MCP 使用远程执行工具；Android 后续增加工作区状态识别及网盘式文件界面。

| 项目 | 本方案确定选择 |
|---|---|
| 产品名称 | Core Admin、Android 和 MCP 展示统一使用“远程工作区” |
| 领域概念 | 沿用路线图中的 `AgentSpace`、`agentSpaceId = spc_<uuid>`，不另建 `RemoteWorkspace` 实体或第二套空间 ID |
| 服务边界 | Agent Space 是独立部署、独立持久化的第三方服务；Core 是集成调用方 |
| 集成管理 | Admin 新增“集成管理”；Agent Space 为首个类型；配置保存、检查、应用和停用无需重启 Core |
| 初始规模 | 首期一个生效的 Agent Space 集成，每个企业用户最多一个未删除的远程工作区；不引入集群调度或多工作区选择 |
| 账号规则 | 新账号使用 `measix_<用户 UUID>`，保留完整 UUID 和连字符；不使用部署 UUID 拼接、Base32 或截断哈希 |
| 开通方式 | 管理员显式开通，不因企业登录或一次 MCP 探测自动创建账号；VM 在实际工具或文件访问时按需启动 |
| MCP 语义 | 复用 Direct Managed MCP；企业启用后用户不能关闭、编辑或删除；不增加 Android MCP 开关 |
| Agent 使用 | 仍按 Assistant 的 MCP 引用装配工具，不向所有助手自动注入 shell，不改写企业固定引用 |
| 断开 | 撤销用户访问并停止运行，保留账号绑定、空间 ID 和文件；恢复继续使用原空间 |
| 删除 | 独立的破坏性操作，明确确认、异步执行、核实资源清理完成 |
| 文件管理 | 目录浏览、元信息、上传、下载、新建目录、重命名、移动、复制和删除 |
| 首期预览 | Markdown、纯文本、PDF、JPEG/PNG/WebP/GIF/BMP；Core Admin 首期交付，Android 文件界面采用同一范围 |
| 数据兼容 | 显式迁移，保留历史用户、空间、凭据、发布和 Android 数据；无自动清库或重建工作区 |

“网盘式”描述界面体验，不新增网盘业务实体。首期不提供回收站、文件版本历史、公开分享、在线协同编辑、全文搜索、后台双向同步、Office/音视频预览或可恢复分块上传。Agent Space 已有的临时网页发布继续存在，但文件预览不依赖它。

S1 路线图还要求 compute/storage usage 和资源限额。现有 VM profile/磁盘限制可以复用，但不等于平台计量和每用户 quota 已完成。本方案交付应称“远程工作区集成”；完整 S1 的剩余要求须保留在阶段清单中，不能用集成验收替代。

## 2. 调研基线与当前实现

### 2.1 仓库及证据边界

| 仓库 | 本轮读取基线 | 当前事实 |
|---|---|---|
| `measix-platform-core` | `ba2ab7a864311059427f96eacf067d87b657ccf3` | 已有 Hub、Relay、Admin、Direct MCP、版本化运行配置、Secret 和迁移机制，无 Agent Space 集成；本轮工作树变更只有本文及 README 入口 |
| `measix-architecture` | `f7547cfe43d665127f82703cc37664f31cad397a` | 已规划 S1 Agent Space；具体实现必须遵守阶段与组件边界 |
| Agent Space 仓库 `/home/hualei/msb_demo` | `ddf83a9b98fb70f0ef3310e1893f7060a09b34f4`，工作树干净 | WebDAV、受管 helper、根目录别名保护及测试/打包门禁均已提交；本轮读取此版本的文档和实现 |

本轮重新读取 Agent Space 的 `AGENTS.md`、`docs/README.md`、架构、接入参考、WebDAV 方案/实施记录、受管进程规范，并核对下列实现：

- `agent-space/src/state.rs`、`workspace_service.rs`、`session.rs`；
- `agent-space/agent-space-mcp/src/admin_api.rs`、`webdav.rs`、`origins.rs`；
- `agent-space/webdav/src/lib.rs`、`fs.rs` 和文件测试；
- `agent-space/docker/package.py`、`verify-migrations.py` 的根保护及发布门禁。

Agent Space 的 `docs/webdav-execution.md` 现记录根路径修复后的固定候选：镜像 `agent-space:schema2-eb5270841c1bea1b`，image ID `sha256:d6ecc1433560096c1e3778606f5c441abe314dd73571754e34e98acac1c68f69`，源码内容 SHA-256 `eb5270841c1bea1be4261a37a8b4ad5e4afe5a910b84fb9f17be99b88b40cd8c`。Git 提交与构建内容摘要是不同标识，不互相替代。

该记录包含完整重跑 **207 passed / 0 failed / 0 ignored**、严格 clippy、12 项 Docker 生命周期、静态用户/schema1 两条历史迁移与回滚、根路径保护及 rclone/FUSE。首轮真实 VM 的 64 MiB PUT 曾返回 SDK `local shared arena connection is closed`/HTTP 502；未修改传输代码后重跑通过，根因未定位、未证明已修复，集成传输验收须保留此风险与原失败证据。

上述为该项目记录的既往验证，本轮文档审查未重跑构建、VM 或客户端测试，也未核验生产部署。GUI 文件操作、浏览器附件落盘仍未验证；既往取消测试不免除本次新增 Admin/Android 界面的验收。旧 206 项测试及较早浏览器隔离证据只适用于各自内容身份，不能替代新集成候选。

### 2.2 可以复用的 Agent Space 能力

| 能力 | 当前实现及限制 |
|---|---|
| 隔离与持久化 | 单节点、每账号独立 microsandbox VM、根盘和 `/workspace` 卷；同用户多设备/多会话共享该空间，不是每会话独立沙箱 |
| 状态权威 | `StateStore` 的 `state.json` 保存账号、凭据摘要、`spc_*` 和精确资源清单；单进程排他写入；缺失/损坏不自动初始化 |
| 管理 API | 创建、查询、启用/禁用、MCP key 轮换、DAV key 轮换/撤销、异步删除；单用户变更核对空间 ID；无独立 MCP key 撤销端点，禁用会撤销两类 key |
| MCP | `/u/{username}/mcp`；五个工具 `bash/read/write/edit/http_proxy`；请求及会话绑定身份，排队执行前复验；初始化和 tools/list 不启动 VM |
| 凭据 | 每账号一个有效 MCP key，另一个独立 DAV key；创建/轮换明文只返回一次；禁用同时撤销两者 |
| 生命周期 | 禁用停止失败保留 `stopPending`；删除持久化意图后重试；未知 backend 状态不当作不存在、不重建磁盘 |
| 文件访问 | DAV 使用同一 `/workspace`、同一 SessionManager 和活动租约；支持 Range、条件请求、锁、常规文件方法 |
| 文件保护 | 完整 PUT 及普通文件 COPY 暂存后原子替换；MOVE 不预删目标；目录递归可能部分完成；DAV 锁不约束任意 bash 写入 |
| 已有增强 | 修改时间、真实卷 used/available、路径修改预约；不支持部分 PUT、持久自定义属性或跨重启锁 |
| 域名边界 | MCP/admin、DAV、每空间临时网页按实际 Host/authority 隔离；不能只改返回 URL 或伪造转发头 |
| helper | guest 中由 Host 分发，具有实例归属、摘要及有界日志；禁止另建第二份 helper 或通配终止用户进程 |

### 2.3 尚缺少的能力

1. Agent Space 当前 `/admin/v1/status` 只提供 ready/activeSpaces 等信息，没有本方案需要的持久服务实例身份、版本化能力查询和管理文件入口。
2. 当前创建用户只有 username，没有可确认“响应丢失的创建属于本次操作”的持久创建关联标识；管理变更也没有账号修订条件检查，空间 ID 相同仍可能发生迟到请求覆盖新意图。
3. 普通 DAV 依赖 active 账号和有效 DAV key，不能直接用于已断开账号的管理员维护。
4. Core Relay 当前按共享 resource → route → upstream 解析地址和凭据，没有每用户工作区绑定。
5. Core 尚无集成管理、工作区状态/文件 API、相应 Admin 页面及 Android 文件投影。
6. Agent Space 接入文档仍约定直接使用 `usr_*`；本文的新账号约定必须同步更新该说明，不能假装现有文档已经一致。
7. Core 的发布绑定、Relay 路由、预算准入和 Usage 事实均依赖 `upstreamId`。独立集成目标需要贯通这些合同，不能只增加用户 Token 表。
8. Relay 当前控制 Store 原子替换内存状态并 ACK；没有本文要求的按工作区主动取消在途流的完整机制，也不持久化整份运行状态。重启后的恢复由 Hub 负责。

### 2.4 关键实现核对位置

以下位置用于开发者复核本方案依赖，不把规划能力写成当前实现：

| 所在仓库/文件 | 本轮确认内容与实施影响 |
|---|---|
| Agent Space `agent-space/agent-space-mcp/src/admin_api.rs` | 现有 v1 路由、创建输入和空间条件；新增 info、创建关联、修订条件及文件入口需独立协议实现 |
| Agent Space `agent-space/src/state.rs`、`workspace_service.rs` | 禁用清两类凭据、stopPending、删除意图；须增加修订校验并限制旧停机完成回写 |
| Agent Space `agent-space/agent-space-mcp/src/webdav.rs`、`agent-space/src/session.rs` | 普通 DAV 身份复验与活动租约；维护模式必须纳入同一 Session 所有者 |
| Agent Space `agent-space/webdav/src/lib.rs`、`fs.rs`、`tests/files.rs` | 根路径保护已实现；文件 ETag 来自 inode/长度/mtime/ctime 元数据，非内容快照 |
| Core `backend/internal/hub/capability/service.go`、`api/admin/admin.openapi.yaml` | RuntimeBinding 当前必须指向 Upstream；需增加显式目标分支 |
| Core `backend/internal/relay/control/store.go`、`backend/internal/hub/runtimecontrol/reconcile.go` | ACK、全局修订及恢复；工作区任务不能另设控制状态写入者 |
| Core `api/internal/usage-ingest.openapi.yaml`、`backend/internal/hub/budget/admission.go` | 准入/用量依赖 Upstream 归属及摘要，必须同步版本化扩展 |
| Core `backend/internal/hub/runtimecontrol/security_change.go`、`user_deletion.go` | Relay 确认后清理用户；远端清理目标必须在该事务删除用户数据前保存 |

## 3. 术语、权威与项目边界

“远程工作区”是产品名称，对应已有 Hosted Workspace / AgentSpace。第三方集成类型仍叫 Agent Space；`AgentSpacePolicy` 是控制定义，`AgentSpace` 是运行实例。文件、VM 状态和用户开通状态不进入共享 Managed Snapshot。

| 事实/行为 | 唯一所有者 |
|---|---|
| 企业身份、用户资格、管理员权限 | Control Hub |
| 集成配置、开通意图、凭据安全保存、操作恢复 | Control Hub |
| 企业 MCP 定义、Assistant 引用及 Release | Core 现有能力发布领域 |
| MCP 用户绑定的应用、鉴权和透明转发 | Runtime Relay |
| 远端账号、空间 ID、凭据有效性、VM/磁盘/文件 | Agent Space |
| 文件准入、管理审计、Client/Admin 文件协议 | Control Hub |
| 文件原子修改、目录边界、guest 文件读取 | Agent Space 现有文件与运行所有者 |
| 文件预览渲染、选择器和传输交互 | Admin / Android |

Core 不读写 Agent Space 状态文件，不调用 microsandbox，不在宿主挂载运行中的用户盘，不维护另一份文件树或 VM 回收器。Agent Space 不识别 Core 的 Release、Assistant、Android Session、Realm 或管理员角色，不读取 Hub 数据库；`measix_` 对它只是合法账号字符。

Core 保存的是接入绑定和操作意图，不是第二份账号权威：显示名仍来自 Core，实际空间/磁盘存在性仍来自 Agent Space。服务故障时保留最近观测和时间，并标记未知，不把缓存当作当前运行事实。

新文件链路选择为 `Admin/Android → Hub 文件 API → Agent Space 管理文件入口`，MCP 保持 `Android → Relay → Agent Space`。Hub 文件流转是本次明确新增的职责；必须在架构阶段澄清现有“Hub 不承载 Runtime body”的边界，保留模型/MCP Runtime 正文走 Relay 的规则。不能未经更新权威就把文件流当成已获允许的现有职责。

## 4. 账号、空间和服务身份

### 4.1 新账号映射

```text
Core:        usr_550e8400-e29b-41d4-a716-446655440000
Agent Space: measix_550e8400-e29b-41d4-a716-446655440000
```

算法是严格验证 Core User ID 后，将固定 `usr_` 前缀替换为 `measix_`，UUID 不改变。结果 43 字符，满足当前 64 字符上限。生成只发生在新绑定创建时，之后从绑定读取 `remoteUsername`。

完整随机 UUID 满足正常独立部署的唯一性要求。前缀标记产品来源，不构成权限隔离；克隆同一 Core 数据库会保留同一用户身份，不能据此认为克隆部署拥有另一批空间。克隆、接管和服务迁移须显式处理，不能自动领养同名账号或按前缀批量删除。

### 4.2 稳定身份

- `integrationId` 标识 Core 的第三方连接配置；其新 ID 前缀须先登记到标识合同，不复用 `spc_*`。
- `serviceInstanceId` 由 Agent Space 初始化/显式迁移生成并持久保存；重启、换域名不改变；恢复同一实例备份保留。
- `agentSpaceId` 由 Agent Space 创建，Core 不再生成一个平行空间 ID。
- 所有管理操作和文件请求固定期望的服务实例与空间 ID；旧请求不自动跟随同名重建。
- 新建独立服务不能复用克隆实例身份。更换存储或迁移服务需要单独的运维流程，不属于修改 URL 的副作用。

不同修订各有一个所有者，不共用计数器：`configRevision` 是 Hub 集成配置版本，`bindingRevision` 是 Hub 用户绑定/授权意图版本，`controlRevision` 是整份 Relay 控制投影版本；新增 `recordRevision` 是 Agent Space 单账号管理事实版本。文件 ETag 只表达文件校验条件，`managedGeneration` 仍只服务既有发布语义。

现有裸 `usr_*` 或人工账号只能通过显式接管流程绑定：核对服务实例、空间 ID、目标企业用户和操作者，再保存原 remoteUsername；不改名、不复制或重建文件。一个服务实例/空间不得同时绑定给两个 Core 用户。

接管时明确转移凭据管理权：先保存接管意图并撤销既有 MCP/DAV 访问、等待停机，再按同一 spc 恢复并签发由 Core 保管的新 MCP key；不向用户交付 DAV key。新建托管账号也不签发供用户直连的 DAV key。一个空间只允许一个控制方持续编排；Core 唯一约束只覆盖自身数据库，不冒充 Agent Space 已提供跨管理方所有权锁。克隆部署必须停用原控制方或完成显式移交，不能双写同一空间。

## 5. Admin 集成管理与热更新

### 5.1 页面与配置

Admin 新增“集成管理 → Agent Space”，提供配置、连接检查、保存并应用、停用、用户空间列表、操作进度和脱敏诊断。用户详情增加“远程工作区”，复用同一后端操作，不创建另一套状态。

公共集成结构只包含 ID、类型、名称、配置版本、启用意图、生效状态和诊断。Agent Space 使用类型化配置，首期不引入动态插件加载、任意脚本、通用 JSON 编辑器或自动发现服务。

运行配置包含管理/MCP 访问地址、管理凭据版本引用、必要的管理请求和文件 IO 超时。文件管理复用管理入口，不要求 Core 为内部文件访问配置公众 DAV 域名。Agent Space 的公开 DAV/临时网页域名继续由其独立部署配置决定。

管理员输入凭据后只返回 Secret 引用及版本；明文不进浏览器持久状态、URL、审计或日志。数据库配置是集成运行权威，不让环境变量覆盖 Admin 修改。主密钥、监听地址、数据库路径及服务自身的 TLS/DNS 仍为部署配置；“热更新集成”不表示远程改写 Agent Space 的 config.toml。

### 5.2 应用协议

1. 更新携带 expectedRevision；先验证字段并保存候选版本，重复提交使用相同幂等键。
2. 后台检查服务身份、接口版本、认证、MCP/文件能力。只读检查不创建测试账号、不启动 VM。
3. 已有该集成的生效运行绑定时，将新目标编译进现有 Hub→Relay 控制投影，以 controlRevision 和 bundleHash 应用；不增加 managedGeneration。
4. Relay 原子应用并返回精确确认；Hub 在持久化确认后更新 activeConfigRevision，并开放与该配置一致的文件准入。
5. Hub 重启或确认丢失时查询已应用版本收敛，不盲目重发旧配置覆盖新状态。

尚无 Release 或集成尚未发布/无运行绑定时，连接检查和配置应用在 Hub 内以数据库事务生效，标记“MCP 待发布/无运行绑定”，不依赖现有要求 activeReleaseContent 的 Upstream 应用入口，不伪造 generation=0 的发布。首个关联 Release 仍由现有 Publish 应用 Relay 控制；发布之前禁止创建并连接新用户工作区。已有工作区在撤下 MCP 后仍可进行文件和生命周期管理。

跨 Hub/Relay 切换不宣称分布式原子事务。发送控制前持久化目标及待应用状态；发生身份、凭据或撤销语义变化时先关闭相关 Hub 文件准入，再应用 Relay，确认后重开。只影响后续请求的普通超时参数变更可让已准入请求按旧配置完成。Relay 重启当前状态为空、拒绝运行请求，Hub reconcile 从持久权威重新下发；ACK 不是 Relay 落盘、远端身份有效或 VM 停止的证明。

复用 `upstream` 的 Secret/配置版本机制和 `runtimecontrol` 的 Activation、幂等与 reconcile 模式，但业务操作留在集成/工作区领域，不把创建 VM 塞进 Upstream 服务。全局控制修订仍由现有唯一写入路径串行分配，不能由每个用户工作流各自覆盖整份 Relay 状态。

远端创建、等待停机或删除轮询不长期占用全局配置 Activation；只有编译/应用控制修订的阶段进入现有串行协议。空间任务先保存自身进度，控制应用忙时等待重试，避免一个离线工作区阻塞所有无关配置更新。真正已发送且结果未知的控制应用仍须先对账，不能为绕过互斥另造控制写入口。

配置校验失败且尚未发送应用时，可确定旧版本继续有效；发送后结果未知时必须标记 APPLYING/UNKNOWN 并对账，不能报告已回滚。新凭据如果已在远端轮换，旧凭据可能已无效，也不能承诺透明退回旧值。

地址变化须匹配原 serviceInstanceId；不同实例拒绝直接覆盖已有绑定。使用配置中的合法 authority/TLS 名称，保留 Agent Space 的入口校验；不能把公网 origin 换成任意内网 IP 后靠 X-Forwarded-Host 绕过。远端返回的 mcpUrl 只能用于校验，不成为客户端任意 URL 或带凭据重定向的来源。

### 5.3 生效与停用

配置状态、连接健康、MCP 是否已发布分别展示。有效配置不等于远端此刻健康；单个集成故障不停止其他 Core 能力。

普通参数切换：新请求使用新版本，在途工作保留原目标。安全变更：关闭受影响请求准入，取消关联连接；用户 Token 轮换会使旧 MCP session 失效，应重新初始化而非重放 tools/call。

停用集成先持久化停用意图，阻断 Hub 文件请求，向 Relay 撤销运行绑定，同时协调停止本集成登记的远端空间。完整停用以 Relay 撤销及远端停用均确认作为完成条件；不可达时显示进行中和哪一侧未确认，不承诺网络分区下瞬时撤销。管理收尾可继续使用受保管的管理凭据，但不再开放新的交互式文件维护。

重新启用只恢复仍期望连接且企业身份有效的绑定，不能恢复用户主动断开或待删除空间。集成关闭保留配置、秘密引用和清理记录；只有相关空间清理/显式移交完毕、MCP 引用解除后才能删除集成记录，避免遗留无人管理的磁盘。

## 6. MCP 发布与每用户运行绑定

### 6.1 一个企业 MCP 定义

每个集成生成一个稳定 `mcpServerId`，展示名“远程工作区”，协议 MCP_STREAMABLE_HTTP，authOwnership 保持 ENTERPRISE_MANAGED。它不是 USER_MANAGED OAuth，也不使用管理员 Bearer 执行用户工具。

首次接入把生成的定义及内部运行关联加入现有能力草稿，经过 Validate/Publish 后可供用户开通。自动生成不等于自动发布所有未完成草稿；Admin 明确显示“集成已配置，MCP 待发布”。不要建立旁路发布，也不要为每个用户复制 MCP 定义、Assistant 或 Release。

集成维护连接和凭据；公开名称/Assistant 引用按现有能力发布权限管理。用户开通/断开和后台凭据轮换只更新用户绑定与 controlRevision，不改变共享 Snapshot。配置更新不改变 mcpServerId；替换独立服务须显式处理。

未开通用户可能仍在共享企业目录看到该 MCP：新 Android 结合工作区投影显示“未开通”，Relay 始终拒绝执行。不得因共享目录可见推断已授权。需要工作区的 Assistant 未获得可调用资源时报告明确准备失败；不静默跳过固定引用或回退本地 shell。未引用该服务的普通 Assistant 不受工作区不可用影响。

当前 Admin `RuntimeBindingDefinition` 强制要求 upstreamId，能力校验和发布编译也直接查共享 Upstream，因此这里需要同步扩展服务器端发布合同，不能仅在 Relay 加一张表。建议增加明确的“集成目标”绑定分支，引用 integrationId，与旧 upstreamId 分支互斥；旧发布继续按原分支解释，新集成定义按服务能力、集成身份和 MCP 路径校验。不要创建一个 Auth=NONE 的假 Upstream 或放入管理员 Token 来绕过校验。分支的 wire 版本、历史 Draft/Release 读取和重新发布规则须在 P0 固定；客户端 McpDefinition 无需暴露此分支。发布回滚和工作区 reconcile 均要以当前有效 Release 为准，不得复活已从发布中撤下的资源。

该绑定分支是必需的服务端合同扩展，不能在保持 upstreamId 必填的旧 DTO 中塞入 integrationId。旧 Upstream 分支保持原字段含义；新分支有显式类型判别和协议支持检查，并覆盖草稿编辑、校验、发布、回滚、控制重建与历史读取。不得为了让旧服务端接受而生成占位 Upstream ID。

### 6.2 Relay 解析与撤销

```text
平台 Bearer → 已验证的 deployment/user/device/session
  → 既有资源和 generation 校验
  → 按 userId + mcpServerId 查找已应用绑定
  → 固定 remoteUsername / spc / endpoint / 用户 Token
  → 标准 MCP Streamable HTTP
```

Hub 编译绑定；Relay 只消费类型化控制投影，不访问 hub.db，也不持有管理凭据。按主体解析是受控的运行绑定能力，不增加任意 URL 模板、脚本、请求体翻译或 JSON-RPC 工具代理。

每用户绑定包含目标身份、明确上游路径、用户凭据及版本；日志/描述 hash 使用 Secret 引用与版本，不泄漏明文。无绑定、已撤销或空间失配一律拒绝，禁止回退到共享 Upstream 的凭据。身份来自已验证 claims，不接受工具参数、query 或客户端自报 Header 选择用户。

POST、GET/SSE、DELETE、协议/session header 全部经过同一绑定；保留标准工具结果和流式取消。准入更新要主动关闭被撤销绑定的在途流，防止只影响下一次请求。活动请求登记只负责取消，不成为第二份 MCP session 或身份权威。旧 MCP session 在同名重建后不可复用。

实现应在读取当前绑定与登记请求之间形成可复核的准入步骤：登记后、真正转发前再次核对绑定版本和 Session 资格；控制撤销同时更新准入并取消旧版本登记，避免“枚举取消结束后才注册”的漏网请求。相同 revision/hash 重复应用也须确认该撤销步骤已执行，再返回成功。用户/设备/Session 撤销和绑定撤销共用请求取消机制。取消不等于已回滚远端 bash 的副作用；需要工作区停止的操作仍等 Agent Space 独立确认。

新内部控制字段必须有版本/能力门禁。旧 Relay 不能忽略它后按共享凭据执行；未升级 Relay 时禁止启用此集成。权限拒绝和绑定失败不走预算/计量降级的放行分支。

公开 MCP 路径只接受发布的固定 endpoint；转发时替换为本用户唯一的 `/u/{remoteUsername}/mcp`，不把客户端任意路径后缀或 query 拼进用户选择。绑定身份、目标、修订及 Secret 版本都进入控制描述/摘要和重建验证。每次新控制编译必须保留其他用户有效绑定，撤销则删除对应准入并结束旧连接；完整状态恢复同样从 Hub 的当前权威重建，不能从旧日志恢复明文凭据。

### 6.3 预算与用量归属

现有 `BudgetAdmissionRequest`、`RequestUsageFact` 和准入校验要求合法 upstreamId；发布与 Relay 改为集成目标后，这条链路也必须同步扩展。选择类型化目标归属：保留历史 Upstream 分支，新分支保存 integrationId 和固定的用户空间/绑定身份；协议版本、确切字段及 Admin 投影在 P0 一起冻结。不把 integrationId 填进 upstreamId，不造虚构 Upstream，不跳过原有 MCP 预算准入。

准入摘要、预算请求持久化、Relay spool、Usage ingest/ledger、请求详情及对账都必须保存请求发生时的不可变目标，不能在集成改名、用户删除或空间重建后查询当前绑定补历史事实。旧 spool/账本按原合同读取，追加迁移保留历史归属；新事件不下发给不支持该分支的消费者。用户未开通等权限拒绝仍由授权层拒绝，不能套用计量降级策略放行。

Direct MCP 计量仍记录转发请求，不等于 JSON-RPC 工具调用次数、VM 时间或存储用量。文件传输记录操作/字节审计，不伪装成 MCP 用量；compute/storage 由 S1 相应合同后续定义。

## 7. 用户生命周期、状态与恢复

### 7.1 状态模型

持久化期望状态采用连接、断开、删除三种意图；当前进度由待完成操作、Relay 确认和远端观测派生，避免多组可独立修改的 enabled/connected/ready 标志。

UI 至少区分未开通、开通中、已连接、断开中、已断开、恢复中、删除中和需要处理的错误。VM 休眠是“已连接”的运行状态，不显示为断开；UNKNOWN 不当作未开通。

连接意图、访问可用性和 VM 状态分开投影。客户端不得只用一个 connected 布尔值同时决定文件和 MCP 权限：

| 条件 | 用户文件 | 用户 MCP | Admin 文件 |
|---|---|---|---|
| 集成生效、用户有效、空间连接收敛，MCP 已发布 | 可用 | 绑定已应用且 Assistant 引用时可用 | 可用 |
| 上述空间保留连接，但 MCP 从 Release 撤下 | 可用 | 不可用，撤销 Relay 绑定 | 可用 |
| 空间已断开且停机已收敛，集成生效 | 不可用 | 不可用 | 显式维护模式可用 |
| 企业用户被禁用且远端停机已收敛，集成生效 | 不可用 | 不可用 | 有权限管理员可显式维护 |
| 停机待完成、删除中或目标身份未确认 | 不可用 | 不可用 | 不可用，仅诊断/收尾 |
| 集成停用或目标服务不可用 | 不可用 | 不可用 | 不可用，仅诊断/收尾 |

撤下/重新发布 MCP 只改变该资源的运行准入，不隐式断开账号、删除文件或轮换凭据；重新发布仍要求当前用户资格和连接意图。Admin 维护不依赖资源是否被 Assistant 引用。

### 7.2 操作次序

| 操作 | 必须执行与完成条件 |
|---|---|
| 创建并连接 | 检查集成/MCP 已发布及用户资格 → 保存创建意图 → 创建远端账号 → 保存 spc/加密凭据 → 应用用户绑定 → 确认生效；创建本身不启动 VM |
| 断开 | 保存断开意图、立即阻断 Hub 用户文件请求 → 撤销 Relay、禁用远端账号并停止 VM → 双侧确认；保留文件 |
| 恢复 | 复验用户资格、相同服务/spc 且停止完成 → 恢复账号 → 轮换并保存用户 Token → 应用绑定 → 确认生效 |
| 删除空间 | 保存原 spc 的删除意图并撤销访问 → 远端 DELETE → 查询原目标至确认不存在 → 清除秘密和活动绑定 |
| 企业用户禁用 | 立即沿既有身份链撤销平台访问，再协调远端禁用；保留原连接意图以区分管理员断开 |
| 企业用户恢复 | 仅恢复原有连接意图且集成仍生效的空间；重新检查远端、轮换凭据，不复活删除任务 |
| 企业用户删除 | 先拒绝身份访问，再以最小独立清理任务完成远端删除；不因级联删除用户行丢失清理目标 |
| 单设备退出/撤销 | 关闭该设备/Session 的请求，保留用户空间和其他设备资格 |

创建、断开、恢复、删除使用 Admin 权限和幂等命令。首期 Android 不提供服务级开通/断开开关；未来自助开通需单独定义授权，不能从 MCP 可用性推导。

### 7.3 创建和凭据的不确定结果

当前创建 API 的“同名返回 409，再 GET”只能证明账号存在，不能证明属于本次创建。新增通用可选 `creationRequestId`：可信管理调用方提交随机关联 ID，Agent Space 与新账号原子保存，管理 GET 可查询；它不携带 Core 产品语义。Core 创建必须提供该值。同一实例内该 ID 只能关联一次创建，不能用于另一 username；活跃账号的重复 POST 可返回冲突，调用方通过该标识判断同一操作。

Core 在发送前持久化该标识。响应丢失时，仅在关联标识匹配且服务/spc 核对成功后接纳账号并恢复凭据交付；无标识的历史账号或标识不符进入显式接管，不自动轮换。不得让创建重试覆盖已有标识。

关联标识不能随账号删除而失去去重效力。Agent Space 在删除账号记录时原子保留最小创建墓碑（已使用的 creationRequestId 摘要），同一 ID 的迟到 POST 返回明确的“创建已结束且不可重用”，绝不重新创建。墓碑不保存文件、凭据或账号显示信息，不扩展为通用任务日志；备份/恢复必须包含它，新一次开通使用新 ID。仅在账号行保存关联 ID 无法防止“删除完成后，旧创建请求到达”产生孤儿空间。

用户 Token 只返回一次。响应到达后须先加密持久化再开放 Relay；持久化失败或轮换响应丢失时保持用户绑定不可用，串行查询/轮换恢复。不通过 GET 返回原明文，不将旧凭据有效性当作回滚保证。每绑定同一时刻只有一个改变凭据的操作；后台 reconcile 不每轮重新轮换。

轮换前关闭该绑定准入并撤销旧 Relay 绑定；成功保存新 Token 后再发布新绑定。先轮换后撤销会让旧 Relay 状态继续接受注定失败的请求。创建中或恢复中若资格/意图改变，丢弃凭据的发布机会并转入最新意图收尾，不能因为远端调用成功而自动连接。

Core 托管账号的 MCP 凭据由 Core 统一管理，不向用户交付后再允许另一套直连配置独立轮换。Agent Space 独立管理员仍可撤销该 key；Core 发现外部变更时标记凭据失效并进入明确恢复流程，不能无限自动轮换与外部管理方竞争。现有服务级管理员凭据并不提供多个互不信任管理方之间的细粒度隔离，此类能力不由账号前缀保证。

### 7.4 删除、重启与并发

所有操作绑定期望集成配置版本和原 spc，旧任务不能覆盖新意图。一次删除确认之后才能重建；重建得到新 spc，旧文件引用和会话失效。404 仅在正确服务身份、正确路由、通过管理认证后才能作为不存在证据，网关 404/不可达不是删除完成。

本地串行操作不能阻止已经发出的 HTTP 请求迟到。新增 Agent Space `recordRevision` 条件变更是必需依赖：Core 每次发送启用、禁用、轮换或删除都携带读到的账号修订；冲突后重新查询并按当前持久意图决策，不改修订后盲目重放旧命令。即使远端已呈现期望的 disabled，也要通过条件写入确认本次禁用意图，使尚未到达的旧 enable 不能再生效。具体所有者和规则见 8.1。

对未收到创建响应却被用户取消的开通任务，先恢复其远端归属信息，再执行断开/清理；不能直接丢弃本地意图。GET 404 不能证明在途 POST 永远不会执行，须用同一 creationRequestId 收敛到已创建的确定账号或明确的创建墓碑，再完成禁用/删除。该补偿可能最终创建一个随即禁用或删除的账号，但不会启动 VM；不能为避免这一步把不确定结果标记完成。已开始的删除不能通过改回连接状态撤销。

操作持久化在 Hub，现有后台循环负责恢复和有界退避；不引入独立队列或按用户创建常驻定时器。管理员重试沿用同一操作目标。文件修改和 bash 的结果未知不能由管理重试机制自动重放。

企业用户删除必须与当前 `finalizeSecurityChange → purgeUserData` 接上：在同一 Hub 事务内先保存独立远端清理记录，再清除用户私有数据，不能依靠删除后已不存在的 User/绑定行继续轮询。清理记录只保留管理目标、期望身份/修订、操作及必要秘密引用，不保存文件列表或正文；清理完成即删除不再需要的数据。已有身份删除的 deny-first、凭据墓碑和预算收尾保持原义。

原用户删除完成表示 Core 身份/私有数据处理完成；远端空间清理另有持久进度，Admin 必须同时显示，不能把原 Activation COMPLETED 解释为磁盘已删除。远端不可达不长期占住全局 Activation，但清理任务未完成前禁止删除其集成/必要管理秘密。此跨域清理记录及完成展示属于必须补入架构的删除语义，不能以无关联后台日志代替。

## 8. Agent Space 的最小通用扩展

这些都是待实现内容，应由 Agent Space 自己维护接口合同、状态迁移及测试。Core 文档只规定集成所需能力，不接管其内部源文件所有权。

### 8.1 服务身份、创建归属与条件变更

新增管理认证保护的服务信息查询（建议 `/admin/v1/info`），包含 serviceInstanceId、管理协议版本、已支持能力及可用于诊断的构建身份。与 `/healthz` 或瞬时 ready 分离。不能按 microsandbox 0.7.2 版本推断应用能力。

serviceInstanceId、creationRequestId/创建墓碑和账号 recordRevision 纳入现有 StateStore 的显式版本迁移；不在启动时随机补值，也不创建第二个可漂移的身份文件。具体下一 schema 号以实施时仓库为准，不改写已发布 schema2 的含义。历史已删除账号无需推测补墓碑；新合同开始接受的带关联标识创建必须遵守 7.3 的去重和删除保留规则。

账号 `recordRevision` 只管理远端账号控制事实，不计每次文件写入或 VM 唤醒。实现要求：

1. 账号创建确定初值；管理读取返回当前修订。管理变更在现有 StateStore 原子写入内同时校验 expectedServiceInstanceId、spc 和 expectedRecordRevision，成功推进修订并返回新值。创建尚无 spc 时校验实例及 creationRequestId。
2. 每次接受新的管理变更均推进修订，包含“当前已 disabled，仍提交禁用”的情况；条件不符返回稳定冲突，不修改状态、不签发凭据、不停止别人的新会话。
3. 后台停止/删除收尾绑定启动该工作的空间及管理修订；旧 stop 完成回调不得清除新任务的 stopPending。仅更新同一任务的进度/错误不推进管理修订。撤销、停止、恢复/维护准入需在现有运行所有者中协调，不能只给 JSON 增加数字。
4. 管理文件租约绑定相同账号修订和明确目的；相关控制变化主动取消旧租约。断开后又恢复到 active，不允许携带旧修订的迟到文件请求复活。
5. 旧独立调用方可继续使用其明确支持的历史协议；通过旧协议接受的管理变更也推进同一个修订。Core 只使用声明支持条件变更的新协议，缺少能力禁止启用，不能静默回退到无条件 PATCH。

新增字段的 wire 版本及严格解析规则由 Agent Space 协议确定；不要求现有 v1 客户端接受突然增加的必填输入。Core 的兼容门禁至少覆盖服务身份、带删除后去重的创建关联、条件管理变更、普通管理文件及 disabled 维护五项能力。

### 8.2 管理文件访问

在管理认证边界新增通用文件入口，目标为 username + expectedAgentSpaceId + expectedRecordRevision，并校验期望服务实例。提供有界目录元信息和文件内容读取/修改。Core 不调用用户 MCP bash 来模拟 ls/cat/文件上传，也不向浏览器分发管理或 DAV key。

复用现有 guest 文件 helper、RootFs、原子写入、路径预约、guest TCP 和活动租约。Host 管理接口将固定操作映射到同一文件执行层；必要的结构化 DAV 编解码留在 Agent Space 内，不对 XML/路径做字符串替换，不创建第二个 helper、文件树或锁服务。独立公众 DAV 入口保持现有语义；管理文件能力有显式启用条件，由服务信息报告，不借配置缺失静默放行。

管理调用区分两种明确授权目的：

- 普通账号文件访问：账号必须 active；Core 用于已连接企业用户。
- 管理维护：可信管理者显式请求，可维护 disabled 且停止收敛后的空间；不改变账号状态，不生成用户 Token，不开放 MCP/DAV。

该区分必须是管理协议中的受验证字段/操作，不是绕过校验的通用布尔开关。客户端传入的授权目的由 Hub 丢弃；只有通过管理员授权的 Hub 调用能请求维护。Agent Space 不验证 Core 管理员身份，但可记录调用方提供的无权限含义的审计关联 ID。

管理维护可能启动 VM，应在 UI 显示。必须审查现有 disabled/session 撤销关系，建立受管理目的约束的短期活动租约，仍由同一 SessionManager 管理容量、helper 和停止。删除、stopPending、服务停用时拒绝新维护；删除取消现有维护。维护结束后按既有空闲规则收敛，账号始终保持 disabled，不能暂时 PATCH active 再改回 disabled。

MCP 和用户 DAV 继续走现有 active/凭据复验；维护权限不能传播成普通 Session 的普遍豁免。复用文件 helper 不等于复用公众 DAV 的开关：独立服务可关闭公众 DAV 而显式启用管理文件能力；内部 helper 必需配置由 Agent Space 校验，不能伪造公众 Host 或临时签发 DAV key。

### 8.3 必须延续的文件合同

根目录禁止删除/移动/覆盖，所有等价根路径都要保护；当前 `ddf83a9` 已提交根路径及尾斜杠别名回归，打包和两条历史迁移验证也要求该证据。新的管理文件映射必须经过同一保护，不能因为未走公众 DAV 路由而绕过。拒绝穿越、编码绕过、symlink、特殊文件和项目保留目录。读取/元信息限于 `/workspace`，不是整个 guest 根盘。

保留 Range、条件头、原子 PUT 和普通文件 COPY/MOVE 保护。递归目录操作返回可辨认的部分失败，不能把 DAV 207 当全部成功。不同路径可并发，冲突等待有界可取消；不能宣称与任意用户 bash 写入互斥。后端状态未知时返回错误，不为文件访问重建空间。

## 9. Core 文件 API、权限与预览

### 9.1 两个入口，一个应用服务

Admin 入口验证管理员角色、Cookie Session、修改动作 CSRF 及目标用户；Client 入口验证原企业 Session，用户身份从认证主体获取，不接受其他 userId。两者共享同一文件应用服务和 Agent Space adapter。

每次文件操作固定集成、用户、spc、原授权上下文及绑定修订。检查集成生效、用户资格、连接意图和目标状态；转发前复核。配置/身份/绑定撤销取消在途请求；切域、登出、重建后旧结果不能写入新视图或新文件缓存。

文件请求登记与撤销使用与 6.2 相同的登记后复验原则；Admin 登出、会话失效和权限收回也取消以该 Admin Session 发起的传输。Hub 发往远端的管理 Bearer 不代表用户永久有权访问，必须持续约束本次转发的主体、目的及远端账号修订。

普通用户不能访问断开空间。管理员可显式维护断开但保留的空间，删除中一律拒绝；关闭整个集成后只保留诊断和收尾，不开放维护。管理员权限覆盖文件内容访问，UI 明确显示目标用户和空间，审计区分操作者与资源所有者。

文件审计记录 actor、目标用户/spc、动作、经规范化的目标路径、开始/终态、字节数及请求关联 ID；记录谁读过/下载过文件，不记录文件正文、Token 或管理凭据。审计路径也属于受限企业数据，仅向有权限的管理员展示；不借日志保留删除用户的完整私有文件索引。Agent Space 只消费不具授权效力的关联信息，Core 负责可核验的企业操作者归属。

### 9.2 文件操作与传输

Core 使用类型化 JSON 元信息/操作请求，以及流式上传下载正文；具体 OpenAPI 先于代码。公开路径使用工作区根内相对路径，空路径只表示根目录；源/目标都经过同一验证。标识和路径不是授权证明，任何 URL/query 都不包含凭据。

| 操作 | 关键约束 |
|---|---|
| 列目录/元信息 | 浅层列举、有界条目与响应；不能假称当前 DAV 支持分页、全树索引或增量同步；超限明确提示细分路径 |
| 上传 | 原始字节流；保留旧文件至提交；明确覆盖策略和条件，不默认覆盖同名文件 |
| 下载 | 支持必要 Range/条件请求、文件名和实际类型；不得向第三方跳转携带凭据 |
| 新建目录 | 同名/父目录不存在等冲突有稳定错误 |
| 重命名/移动/复制 | 仅同一空间；两端路径和 overwrite 策略固定；跨空间操作不在首期范围 |
| 删除 | 根目录不可删；递归删除确认范围并报告部分失败，结果未知先查询，不自动重放 |

元信息至少包含相对路径、条目类型、文件长度、修改时间及可用 ETag；目录容量使用服务实际提供的卷 used/available，不将其显示为 Core 用户 quota。元信息缺失时明确缺省，不能合成假的创建时间或文件摘要。修改请求显式区分“不覆盖”“按指定版本覆盖”；条件值由文件元信息获得，与账号 recordRevision 分开传递。

操作结果统一区分未执行的拒绝、成功、递归部分成功和提交结果未知。部分成功返回有界的失败路径/原因及是否截断，不能用单一 success 或 207 状态掩盖子项失败。传输中断后的文件提交可能已发生；客户端先刷新目标元信息，不能仅因取消便提示“未保存”。原子 PUT/COPY 只保证失败不留下半个替换文件，不保证已经提交的操作可撤销。

Hub 不整文件缓冲、不先落盘，使用背压和主动取消；不得沿用预览的 1 MiB 正文限制或 bash 的总执行超时。分别限制控制正文、传输并发、连接/启动时间和无进展 IO 时间。上限由实施仓库按真实测试确定并纳入交付配置，超限要有稳定响应；不靠任意增大缓冲解决大文件问题。

上传进度区分已发送与远端提交；正文发送完毕不等于保存成功。下载成功需要实际完成消费/落盘。慢速持续传输应保持活动租约，断流释放资源；响应头后失败终止流，不能把 JSON 错误拼接进文件内容。

### 9.3 文件预览

| 类型 | 首期行为 | 边界 |
|---|---|---|
| Markdown | 渲染阅读、原文切换、代码块与基本排版 | 禁止脚本/危险 HTML；不执行嵌入内容 |
| Text | 文本阅读；代码、日志、JSON 可按纯文本显示 | 首期明确支持 UTF-8/BOM；未知编码或二进制提示下载，不静默猜测成功 |
| PDF | 分页、缩放及加载状态 | 应用内受维护阅读组件，不发送给第三方转换服务，不执行 PDF 脚本/自动动作 |
| 图片 | JPEG、PNG、WebP、GIF、BMP 查看与缩放 | 校验真实格式和解码尺寸；GIF 至少可显示首帧，动画支持须另有客户端证据 |

未支持格式提供下载。SVG、HTML、Office、音视频不作为首期可执行预览。预览失败不修改原文件；大文本、超大图片或资源超限提示限制并提供下载，不一次性无限加载。

Admin 已有 `useMarkdown.ts` 基于 marked/DOMPurify 的企业动态子集，当前会转义图片、限制标签。复用依赖和安全处理思路，新增文件预览的明确渲染模式；不能直接放宽原企业动态渲染器，影响已有安全合同。相对图片在当前文档目录内解析并通过授权文件接口加载；外部图片默认不自动加载，链接显式交互打开，拒绝 javascript 等危险 scheme。

Core 提供内容/元信息，Admin 和 Android 各自渲染。浏览器读取经认证内容后交给受控阅读器；原始文件下载仍 attachment，禁止把任意上传 HTML 作为 Admin 同源文档打开。PDF Range 和 Markdown 图片每次请求都鉴权；304、ETag 命中不能绕过权限。多段读取文件版本变化时重新加载或报冲突，不拼接两个版本。

现有 ETag 是文件元数据校验值，不是不可变内容快照；DAV 锁也不限制 bash 改写文件。Range/条件请求只能发现可检测的变化，不能承诺并发 bash 写入时获得一致版本。受控阅读器可完成一次有界下载后阅读该副本，这只保证后续查看不再随远端变化，不能证明下载期间没有并发改写。发现文件变化时提示等待写入结束再重试；超过客户端资源上限时提供下载，不为首期新增远端快照系统。

预览缓存按企业主体、spc、路径和文件版本隔离，使用 no-store 等适当响应约束；关闭阅读器撤销 blob URL、取消请求并释放临时资源。已下载或已展示的字节不能被远程撤销，访问撤销保证后续读取和未完成传输停止，不承诺收回已交付内容。

文件预览不调用 `http_proxy`、不启动静态网页发布、不生成匿名 URL。Agent 自行发布应用仍走现有独立预览 origin，不与 Admin Cookie origin 合并。Agent Space 不新增 PDF/Markdown 转码、缩略图任务或另一份文件副本。

## 10. Android 企业能力适配

分两步交付，但接口第一步按最终授权边界设计：

1. 工作区识别：查询本用户远程工作区状态、spc、对应 mcpServerId、文件能力与不可用原因；展示企业托管来源。继续由现有 Enterprise/MCP owner 创建连接，不写入个人 Settings，不保存远端 Token。
2. 文件界面：企业域内目录导航、上传下载、基本操作与四类预览；文件选择器上传和下载到用户选定位置；进度、取消、失败、冲突和存储不足可见。

UI 名称为“远程工作区 → 文件”。个人域不注册企业 MCP 或展示企业文件。切企业/退出/Session 撤销沿既有 owner 取消传输和阅读任务；异步结果始终核对原域、原主体和原 spc，不从当前页面反推授权。

文件进入聊天附件时通过现有 Artifact/附件导入链；远程路径不冒充本地已存在文件。手机文件必须上传确认后才能作为 `/workspace/...` 给远端 Agent；MCP 工具与本地 workspace 工具保留明确的执行位置，不因接入服务静默切换本地 shell 或同步 Skills。

工作区状态缓存只用于显示；离线不允许使用陈旧授权访问。Room/DataStore 如有新字段必须显式迁移；不清空企业聊天、个人文件或既有 MCP 目录。Android 的工具定义冻结和执行时授权复验继续遵守现有 Turn/MCP 协议。

## 11. 协议面与兼容策略

### 11.1 拟新增/扩展的接口

以下为职责与路由草案，不是当前可调用 API；最终方法、枚举、错误和 schema 在架构审定后由 OpenAPI 固定，不在前后端手写平行 DTO。

| 接口面 | 拟增加内容 |
|---|---|
| Core Admin `/api/admin/v1/integrations` | 集成创建/查询/更新、check、apply、disable，预期版本和幂等操作引用 |
| Core Admin 用户工作区子资源 | 开通、断开、恢复、删除、状态/进度、显式接管旧空间 |
| Core Admin 工作区文件子资源 | 目标用户文件列表/元信息、内容、目录与修改操作、管理员维护 |
| Core Client `/api/client/v1/workspace` | 单一用户工作区投影及其文件子资源，身份从 Session 获取 |
| Core 能力发布管理面 | RuntimeBinding 的 Upstream/集成目标互斥分支及历史 Draft/Release 支持 |
| Hub→Relay 内部控制 | 显式支持按用户的运行绑定、凭据引用/值及修订，应用/撤销确认 |
| Relay→Hub 用量/预算面 | 集成目标的准入、归属、摘要和用量事实；旧事件读取及 spool 重放兼容 |
| Agent Space 管理面 | info、创建关联标识、带期望修订的管理变更、通用管理文件及维护目的、每请求实例检查 |

工作区投影含独立 schemaVersion、当前绑定状态、spc（未创建时缺省）、已发布 mcpServerId、分别可判定的 MCP/文件可用性及原因、文件能力、状态修订和观测时间；不返回管理员/用户 Token、真实私有上游地址或磁盘名称。它只引用 MCP 定义，不拥有第二份工具 schema/配置。字段缺省与 null 的语义、未开通/集成关闭/远端未知示例须进入共享 fixtures。

管理命令返回可查询的持久操作引用；HTTP 202 只表示意图已接受，客户端轮询该操作和当前投影，不以请求完成时间推导生命周期完成。读取、目录和文件正文使用独立响应，业务错误沿 Core Problem 合同；开始发送文件后发生错误则终止流。创建关联 ID、Core Idempotency-Key、账号条件修订分别解决创建归属、重复命令和并发顺序，不能互相替代。

### 11.2 不改写既有 Snapshot

保留 Client API v1 现有端点语义、Snapshot v4/v5 及历史不可变 Release 字节/hash。新增可选能力通过独立接口获取，不向旧严格 DTO 中直接插入字段，也不为单个用户开通重写共享 Snapshot。Admin 发布绑定、内部控制和预算/用量的新增分支是明确的协议变更，须更新 baseline、版本/能力门禁及支持矩阵，不能用“客户端 Snapshot 不变”声称所有 API 无变更。Portal Bridge 此次无变更；文件能力不通过 Portal Cookie 或 Bridge 偷渡授权。

新 Android 在已确认的 Core 来源查询新增工作区端点；只有成功校验该接口的版本化投影后才启用新功能。新 Core 无工作区应返回结构化未开通状态，不能与未知路径混淆。旧 Core 没有新增能力的权威否定响应，单独一个 404 无法可靠区分旧路由和代理故障；此时保持“未确认支持”、不注册文件功能，保留诊断并允许后续重试，既有企业功能继续运行。401/403 沿身份错误处理，超时/格式错误表示不可用，不写成永久未支持。若增加 Discovery 字段，必须另做旧消费者验证，首期不依赖该修改。

| 组合 | 支持边界 |
|---|---|
| 新 Core + 集成关闭 | 旧功能维持原行为，无远端账号副作用 |
| 新 Android + 旧 Core | 无远程工作区界面，既有企业能力继续工作；新 APK仍支持旧 Snapshot |
| 旧 Android + 新 Core | 既有 Snapshot 可解析；无新文件界面；通过现有 MCP 调用工作区须有真实旧版本协议测试才能列入兼容名单 |
| 新 Core + 旧 Relay | 普通旧配置可按既有支持范围运行；禁止启用按用户绑定，不能忽略扩展降级转发 |
| 新 Core + 旧 Agent Space | 检测为缺少集成能力，保存配置/展示升级要求，不宣称完整就绪 |
| 新 Core + 新 Agent Space + 新 Android | 完整工作区识别、MCP 和文件闭环 |

一般网络错误与权限错误保留稳定 reason/code，不以自然语言解析。建议单独表达未开通、已断开、集成不可用、实例不匹配、维护/删除中、条件冲突、容量不足、结果未知；最终代码集中在对应合同。既有 user_disabled/device_revoked/session_revoked/enterprise_identity_deleted 与 generation barrier 不改变含义。

错误处理必须覆盖：账号修订冲突重新查询意图；文件 ETag 冲突让用户确认覆盖；空间失配停止自动操作；管理认证失败保留配置但拒绝继续执行；远端未知状态不重建；目录部分失败保留已完成项；响应丢失保留操作待核实。Agent Space 原始诊断按服务端关联 ID 保存，客户端只见脱敏的稳定分类。

## 12. 持久化、升级与恢复

### 12.1 Core 数据

建议新增的持久结构仅覆盖真实新事实，命名由实现仓库确定：

- Integration 与不可变配置修订：类型、候选/生效配置、Secret 版本引用、固定 serviceInstanceId。
- 用户工作区绑定：userId、integrationId、remoteUsername、spc、连接意图、凭据引用、绑定修订、远端 recordRevision 观测及已应用控制修订。
- 未完成操作：请求关联、原目标/预期修订、进度、最近错误和恢复信息；尽量复用既有 Activation/Idempotency，空间远端操作不能冒充单次 Relay 应用已完成。
- 预算/用量的目标归属：增加集成分支所需字段和约束，历史 upstreamId 不改写；不以重新计算旧事件/hash 迁移数据。

设置用户未删除空间唯一约束、服务/空间归属唯一约束和并发版本检查。企业用户删除所需清理记录必须与用户行级联生命周期分离，完成后删除其私有数据，仅保留合同允许的审计/不可逆身份墓碑。

严格遵守追加式 SQL migration：不编辑旧文件，Ent 与 SQL 同步；升级默认无启用集成，不批量创建空间。用户、设备、Session、Secret、Draft、历史 Release/hash、Usage 和预算保持。数据库配置引用历史 secretVersion，不能读取“最新密钥”悄悄改变已应用状态。

### 12.2 Agent Space 状态和账号迁移

从支持的历史静态用户/schema1/schema2 起点明确迁移链：先按现有手册完成动态状态转换，再进入新增服务身份/创建关联标识/账号修订版本。备份覆盖 state、根盘和 named volumes；不对缺失文件执行 init-state。静态用户导入按现有合同保留实际磁盘资源、生成空间 ID 并重新签发凭据，不承诺保留静态旧 Token；已有动态 schema1/schema2 升级保留账号、spc、磁盘和既有凭据有效性，除非执行明确的轮换/撤销。历史账号通过一次显式迁移确定 recordRevision 初值；缺少 creationRequestId 的账号走接管而非自动认领。

新 `measix_<uuid>` 只用于新开通账号。Agent Space 现有架构/接入参考中的裸 usr 约定应更新为“由集成方决定账号键”，MEASIX 具体映射留在 Core 适配文档，并保留历史接管说明。

### 12.3 部署次序与回滚

推荐顺序：备份并验证恢复能力 → 升级 Agent Space 接口/状态（不自动启用 Core 集成）→ 升级兼容的新 Relay → Core migrate/check → 启动新版 Hub/Admin → 验证原功能 → 配置并检查集成 → 首次发布 MCP → 显式开通测试用户 → Android 适配/兼容验证 → 扩大开通范围。

Core 继续显式执行 `control-hub migrate`、`control-hub check`，Hub 启动不自动修改 schema。部署包声明最低组件协议能力，具体命令和受支持起点由发行手册固定；不能依赖某个会变化的分支或相同 SDK 标签。

功能回退优先停用集成并保留数据。二进制回退必须确认旧程序能否读取新 schema；否则停服恢复经验证的一致备份。Core 备份包含数据库和必要密钥，Relay spool 按自身所有者处理；Agent Space 备份包含状态与双盘。恢复后核对撤销、轮换、删除和新建事实，不能因恢复旧状态复活旧 Token 或把 Core 绑定指向另一空间。

两项目不要求锁步发布，只按版本化接口协作。升级/恢复测试使用真实代表性历史数据，不以空库通过代替，也不把本地候选验收当成生产验收。

## 13. 文档与代码落点

| 所在仓库 | 本次应更新内容 |
|---|---|
| measix-architecture | 术语中的远程工作区产品名与新集成标识；S1 接入合同；Hub/Relay 的按用户绑定和文件职责；Admin/Android 工作流；版本/迁移/撤销语义及测试要求；阶段索引 |
| Core | Admin/Client/Relay/Usage OpenAPI、fixtures、generated artifacts、协议 baseline；Ent/追加迁移；集成与工作区服务、Agent Space adapter、runtimecontrol/Relay、预算/Usage 归属、文件 API；Admin UI、预览与操作诊断 |
| Agent Space | info、创建关联标识、账号条件变更、管理文件访问及维护租约；StateStore 显式迁移；现有 Session/helper/RootFs 内复用；独立协议文档、发布身份与验证 |
| Android | 导出合同的实际 Kotlin 消费、Enterprise 工作区投影、既有 MCP 接入、文件/预览 UI、退出取消、必要持久迁移和真机验收 |

Core 的具体包拆分保持直接：集成配置、工作区编排及一个 Agent Space HTTP adapter；文件 UI 经应用服务访问。不预先创建 provider 插件框架、通用任务平台或另一套配置总线。

先在架构层冻结本文涉及的新增语义，再落 executable 合同。方案中的路径/字段草案不能被当作绕过该顺序的接口权威。已实现后把事实归入对应当前参考文档，本文保留实施决策和交付记录，避免长期与运行文档重复维护。

## 14. 实施阶段与验收

| 阶段 | 交付内容 | 阶段退出条件 |
|---|---|---|
| P0 合同 | 术语、项目边界、热更新、生命周期、目标绑定及预算/用量、文件/预览、兼容和迁移 | 架构与本方案无冲突；冻结接口/共享案例及组件门禁；不遗留“断开即删除”等歧义 |
| P1 Agent Space | 服务身份、创建关联、账号条件变更、管理文件访问与状态迁移 | 独立服务回归、迟到请求拒绝、维护不开放用户权限、根保护及真实 VM 验证 |
| P2 Core 基础 | 新表/迁移、集成 Admin、Secret、check/apply/reconcile | 无重启切换、冲突/未知结果恢复、原功能不回归 |
| P3 用户与 MCP | 用户操作、发布关联、每用户 Relay 绑定及撤销、预算/用量归属 | 两用户真实 MCP 隔离、原 spc 保留、旧会话失效、幂等恢复、准入与用量闭环 |
| P4 Admin 文件 | 文件 API、基本管理、四类预览、审计 | 真实浏览器上传/落盘/预览及权限测试，不只验证目录可见 |
| P5 Android | 状态识别先交付，随后文件与预览界面 | 新旧组合、真实设备传输、切域/退出、附件闭环和持久迁移 |
| P6 升级交付 | 历史部署演练、备份恢复、发布包和证据 | 固定源码/镜像/合同/客户端版本的完整证据及明确限制 |

P0 是合同落地工作，不是实施中任意选择语义的占位阶段。退出前必须确定：新管理 API 的版本/方法/路径与幂等、修订和错误 schema；目标绑定与预算/用量分支的版本；文件元信息/部分结果/条件头与传输限制配置；客户端投影/旧服务探测；用户删除清理记录；各历史迁移起点。普通库选型、包名和具体 UI 排版由各实现仓库决定。P2 可先完成配置保存和检查，但端到端启用必须等待 P1/P3 对应合同与执行能力齐备。

以下矩阵是本实施方案检查清单，不冒充已登记的架构测试 ID：

| 范围 | 必测场景 |
|---|---|
| 独立性 | Agent Space 无 Core 可启动和使用；Core 未配置集成时原功能可用；Core 不接触远端状态/磁盘 |
| 热更新 | 配置 CAS、保存未应用、无 Release 时首次配置、身份不匹配、认证失败、Relay 已应用但确认丢失、Hub/Relay 重启、普通切换与安全撤销不同处理 |
| 账号/归属 | 固定前缀、两个独立 Core 部署用户不冲突、同名既有账号拒绝自动接管、历史账号接管、数据库克隆冲突明确处理 |
| 开通恢复 | POST 响应丢失、未知创建被取消、删除后旧 POST 重放被墓碑拒绝、同一创建 ID 不可换 username、Token 保存失败、轮换响应丢失、开通中断开、迟到 enable/轮换遇新禁用被拒、重复禁用推进修订、旧 stop 回调不得清除新待停止意图、重启继续操作 |
| 发布与计量 | MCP 撤下只撤销 MCP、重新发布不复活已断开绑定、首次发布不夹带无关草稿、预算拒绝与 Usage 归属、旧 spool 重放、新旧目标历史读取 |
| 用户隔离 | A 的 MCP session/path/文件请求不可访问 B；伪造 userId/Header 无效；无绑定绝不使用共享凭据 |
| 删除 | 删除 202 与完成区分、部分资源清理失败、原 spc 失配、同名重建、删除用户与保存清理目标事务失败/恢复、用户已删仍能收尾且展示远端进度 |
| 管理维护 | disabled 空间文件可受控维护而 MCP/DAV 仍拒绝；stopPending/deleting 拒绝新维护；删除取消维护；结束后回收；公众 DAV 关闭但管理文件显式开启 |
| 文件安全 | 根与别名保护、编码穿越、symlink/特殊文件/保留区拒绝、跨空间目标、条件覆盖、递归部分失败 |
| 文件可靠性 | 至少 64 MiB 往返摘要、慢传输、Range/ETag、磁盘不足、提交前断流保留旧文件/提交后响应丢失结果未知、取消等待/在途 IO、Hub 内存有界；保留 shared arena 失败诊断 |
| 预览 | 四类格式真实打开；Markdown 原文/相对图片与恶意链接；图片解码限制；PDF 多页/Range/版本变化；HTML/SVG 下载不执行 |
| 访问撤销 | 撤销与登记竞态、旧文件请求在断开再恢复后迟到、预览/PDF 分段和传输期间撤销；Admin 失效/客户端切企业与登出；旧视图迟到结果；授权先于 304 |
| 协议兼容 | v4/v5 历史 Snapshot 字节/hash 不变；新端点在旧 Core 缺失；旧 Android 实际 MCP；旧 Relay/Agent Space 禁止伪兼容 |
| 数据迁移 | 空库与各支持历史版本升级、重复迁移、非法历史拒绝、用户/文件/凭据/发布保存、一致备份恢复和旧版本回退 |

代码行为变更按各仓库要求执行 Red → Green → Refactor；文档变更不造人工业务测试。Agent Space 真实 VM 验证先构建两个当前 helper，再串行运行，避免多套清理和构建争用。Core 使用真实 SQLite/HTTP/Relay 与确定性夹具验证恢复，再执行生产构建 Admin 的浏览器闭环；Android 生成类型不等于实际客户端已适配。

每阶段记录 exact commit、未提交输入摘要（如有）、OpenAPI/fixture hash、Agent Space 镜像与 helper 身份、APK/浏览器版本、执行命令、结果及未测项。当前 Agent Space 207 项测试和 Docker/rclone 记录不能替代新增集成能力的候选验证；不得据此宣称生产或完整 S1 通过。

## 15. 参考与待同步文档

### Core / 架构

- [Core 实现架构与文档治理](../ARCHITECTURE.md)
- [API、生成与版本合同](api-contracts.md)
- [数据库迁移](database-migrations.md)
- [Android 平台接入](android-platform-integration.md)
- [Admin 实现](admin-console-implementation.md)
- [测试与证据边界](testing.md)
- [阶段索引](../../measix-architecture/docs/measix-stage-document-index.md)
- [路线图与 S1](../../measix-architecture/docs/00-platform/measix-agent-platform-roadmap.md)
- [术语与标识](../../measix-architecture/docs/00-platform/measix-platform-terminology-and-identifier-contract.md)
- [企业体验生命周期及 Direct MCP](../../measix-architecture/docs/00-platform/measix-enterprise-experience-lifecycle-architecture.md)
- [Runtime Foundation](../../measix-architecture/docs/10-runtime-foundation/measix-runtime-foundation-architecture.md)
- [现有控制协议](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md)

### Agent Space 独立仓库

下列路径相对 `/home/hualei/msb_demo`，不复制该项目的完整接口或运维手册到 Core：

- `docs/agent-space-architecture.md`：当前状态、运行与入口权威。
- `docs/hosted-workspace-reference.md`：现有管理/MCP/DAV 合同；需同步新账号约定及新增接口。
- `docs/workspace-webdav-plan.md`：文件操作、原子性、取消、迁移合同。
- `docs/managed-guest-services.md`：helper 分发、实例归属和升级边界。
- `docs/webdav-execution.md`：具体候选验证身份及未验证客户端范围。
- `docs/docker-upgrade-schema2.md`、`docs/docker-deployment.md`：历史升级、备份与运行流程。

Android 实施前重读其 `AGENTS.md` 和 `docs/references/mcp-architecture.md`、企业配置/持久化/附件相关当前参考文档。本文不取代 Android 的 owner 和迁移合同。
