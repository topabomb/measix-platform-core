# 远程工作区与 Agent Space 集成实施方案

状态：**Core / Relay / Admin 已落入当前候选源码**。本文保留产品选择、独立服务边界和验收要求；具体实现与恢复操作见 [实现参考](remote-workspace-implementation.md)，当前执行证据见 [联调记录](remote-workspace-verification.md)。Android P4 与完整 S1 compute/storage 计量不在本次 Core/Admin 交付范围。

调研日期：2026-09-28；按独立服务接入约定复核：2026-09-29。本文属于 Core 的实施规划；跨组件语义变更必须先落入架构权威，再按 OpenAPI → fixtures → generated artifacts → tests → implementation 实施。跨组件边界以 [S1 远程工作区合同](../../measix-architecture/docs/10-runtime-foundation/s1/measix-s1-remote-workspace-contract-spec.md) 为准，也不修改历史发布内容。已落地 HTTP 方法、字段和生成类型以仓库 OpenAPI 为准；Agent Space 仅使用已有接口。

## 2026-09-30 资源摘要执行计划

1. 复核 Agent Space `661d20d8bfe1fb7630a879383257e61602fb6df6` 的合同与测试，构建隔离联调实例。保留第三方原有修改，不改其实现。该版本单用户 GET 自动采集；Core 普通投影与列表不增加远端查询。
2. 先补 S1 观测边界，再增加管理员 `workspace/resources?agentSpaceId=` 的 OpenAPI、当前/历史/失败 fixture 和生成类型。Client/Snapshot 不变，无数据库迁移或旧版回退。
3. 测试驱动 Core：只读管理适配、字段校验、空间/配置及管理员权限前后复验；允许服务关闭和空间断开后的核查，错误不改变生命周期或文件权限。
4. 共用资源组件：运行状态、磁盘已用/可用、客户机内存已用/配置上限、CPU 核数与配置来源。放在状态操作之后、文件之前；未开通隐藏，历史/缺失/错误分别表达。首次加载、手动刷新、可见时 15 秒串行刷新，切主体或隐藏取消；列表不采集。
5. 验证合同、服务竞态、UI 零值/部分失败/历史/迟到响应/轮询清理；运行后端、前端、类型和构建检查。真实联调覆盖新空间无 VM、文件写入后的占用变化、停止历史、恢复和删除重建，回归文件与 MCP 流程。
6. 实际浏览器检查两个入口、宽屏/窄屏、刷新及生命周期操作、异常提示，修复问题。同步实现文档、版本约束、协议基线和可复现证据，不将本地验证表述成生产部署。

## 1. 目标、范围与确定选择

Core 将“远程工作区”作为可选企业能力。Admin 可以配置并热更新 Agent Space 集成，为指定企业用户创建、断开、恢复或删除远程工作区，并管理其中的文件。开通用户通过现有企业托管 MCP 使用远程执行工具；Android 后续增加工作区状态识别及网盘式文件界面。

接入结论：当前使用 Agent Space `661d20d` 的管理、资源摘要、MCP 和 WebDAV 能力；确切版本与验证身份见 [实现参考](remote-workspace-implementation.md) 和 [联调记录](remote-workspace-verification.md)。目录/文件管理使用 WebDAV；账号创建、启用/禁用、删除及凭据管理使用管理 API；执行工具使用 MCP，无需为 Core 或 Android 另加文件服务或状态协议。Core/Admin 集成已实现并单独验证；Android 后续界面仍需实施，不能把远端能力齐备或本地联调通过等同于生产部署已交付。

| 项目 | 本方案确定选择 |
|---|---|
| 产品名称 | Core Admin、Android 和 MCP 展示统一使用“远程工作区” |
| 领域概念 | 沿用路线图中的 `AgentSpace`、`agentSpaceId = spc_<uuid>`，不另建 `RemoteWorkspace` 实体或第二套空间 ID |
| 服务边界 | Agent Space 是独立部署、独立持久化的第三方服务；本次不修改该项目，Core 复用现有管理 API、MCP 和 WebDAV |
| 管理入口 | Admin 使用“远程工作区”，路由、代码领域名使用专门语义；启用开关在“保存配置”后生效，通用第三方集成管理留待后续 |
| 初始规模 | 首期一个生效的 Agent Space 集成，每个企业用户最多一个未删除的远程工作区；不引入集群调度或多工作区选择 |
| 账号规则 | 新账号使用 `measix_<用户 UUID>`，保留完整 UUID 和连字符；不使用部署 UUID 拼接、Base32 或截断哈希 |
| 开通方式 | 管理员显式开通，不因企业登录或一次 MCP 探测自动创建账号；VM 在实际工具或文件访问时按需启动 |
| MCP 语义 | 复用 Direct Managed MCP；企业启用后用户不能关闭、编辑或删除；不增加 Android MCP 开关 |
| Agent 使用 | 仍按 Assistant 的 MCP 引用装配工具，不向所有助手自动注入 shell，不改写企业固定引用 |
| 断开 | 撤销用户访问并停止运行，保留账号绑定、空间 ID 和文件；恢复继续使用原空间 |
| 删除 | 独立的破坏性操作，明确确认、异步执行、核实资源清理完成 |
| 文件管理 | Core 后端适配现有 WebDAV，提供目录、元信息、上传下载、建目录、重命名、移动、复制和删除；禁用账号或关闭 DAV 时不可用 |
| DAV 直连 | Admin 可查看/复制并交付连接信息，用户可使用外部 WebDAV 客户端；首期不做用户自助领取、审批或临时凭据 |
| DAV 凭据 | Core 生成随机 Token，复用 Secret 加密保存后通过管理 API 设置；确认成功才成为有效凭据，不复用登录凭据或引入派生密钥体系 |
| 首期预览 | Markdown、纯文本、PDF、JPEG/PNG/WebP/GIF/BMP；Core Admin 首期交付，Android 文件界面采用同一范围 |
| 数据兼容 | 显式迁移，保留历史用户、空间、凭据、发布和 Android 数据；无自动清库或重建工作区 |

“网盘式”描述界面体验，不新增网盘业务实体。首期不提供回收站、文件版本历史、公开分享、在线协同编辑、全文搜索、后台双向同步、Office/音视频预览或可恢复分块上传。Agent Space 已有的临时网页发布继续存在，但文件预览不依赖它。

S1 路线图还要求 compute/storage usage 和资源限额。现有 VM profile/磁盘限制可以复用，但不等于平台计量和每用户 quota 已完成。本方案交付应称“远程工作区集成”；完整 S1 的剩余要求须保留在阶段清单中，不能用集成验收替代。

## 2. 原始调研基线与第三方依赖

### 2.1 原始调研记录（历史证据）

本节保留 2026-09-28/29 实施前的基线，表中的“本轮”指当时的方案复核；当前 Core 源码及新增联调证据见实现参考，不用本节替代。

| 仓库 | 本轮读取基线 | 当前事实 |
|---|---|---|
| `measix-platform-core` | `36e2a1a0de2dc4a30e7832c0d193362e9f69d38d` | 已有 Hub、Relay、Admin、Direct MCP、版本化运行配置、Secret 和迁移机制，无 Agent Space 集成；本次修订为文档工作 |
| `measix-architecture` | `f7547cfe43d665127f82703cc37664f31cad397a` 及当前工作树 | S1 合同、术语和阶段入口有未提交规划；本次同步收敛为使用 Agent Space 现有能力 |
| Agent Space 仓库 `/home/hualei/msb_demo` | `3ea01c167fb263f8ef2467b5fe3103353f9a5ddc` | 管理 API 指定 DAV token、同值设置及静态用户升级流程已提交；源码、固定镜像和离线包已完成下述独立验证，未据此宣称生产已升级；本次只读核对 |
| Android `rikkahub_mcp` | `0e1b10e9bb324cd1a13342f47daf0fad04d7b972` | 已有托管 MCP、企业 Session 操作租约及 Artifact 应用入口；本次只读核对，工作区投影/文件界面尚需按本方案实施 |

原方案调研覆盖 Agent Space 的 `AGENTS.md`、`docs/README.md`、架构、接入参考、WebDAV 方案/实施记录、受管进程规范，及下列实现；本次按稳定提交重新核对管理路由、账号/凭据规则、生命周期、DAV 授权/传输、文件条件与提交、升级边界及验证产物：

- `agent-space/src/state.rs`、`workspace_service.rs`、`session.rs`；
- `agent-space/agent-space-mcp/src/admin_api.rs`、`webdav.rs`、`origins.rs`；
- `agent-space/webdav/src/lib.rs`、`fs.rs` 和文件测试；
- `agent-space/docker/package.py`、`verify-static-upgrade.py` 的根保护及发布门禁；当前部署入口为 `docs/docker-deployment.md`。

Agent Space 的 `docs/webdav-execution.md` 对应当前固定版本：镜像 `agent-space:schema2-f977cdaf1d2beef9`，image ID `sha256:150b71a5a19d5db42fcee85fa4945670af7cbd65ce69c7869f14fc20131f4216`，构建输入 SHA-256 `f977cdaf1d2beef90c6031a20cc86308e08b466bba488cd4352b24d10b949602`。本轮在 WSL 中运行 `agent-space/docker/release_identity.py` 重算摘要，并检查本地 Docker 镜像 ID/标签，均与上述记录一致；WSL Git 工作树干净。镜像在提交前构建，OCI revision 标签仍为 `ddf83a9`，但构建输入与当前 `3ea01c1` 一致；不能仅凭该 revision 标签判断是否支持指定 token。Git 提交、构建输入摘要和镜像 ID 是不同标识，不互相替代。

已核对该版本的报告：完整串行测试 **207 passed / 0 failed / 0 ignored**、全目标 Clippy `-D warnings`；`as-verify-639c17b2ae/result.json` 的 12 项 Docker 生命周期（含 310 秒空闲）；`as-upgrade-18726331fd/upgrade.json` 的确切已发布静态镜像升级、13 项检查、独立卷回滚及同镜像 rclone/FUSE（含 64 MiB 往返摘要）。报告均位于 `agent-space/target/docker-verification/`；测试汇总及 helper/源码/离线包身份记录位于其 `webdav-custom-f977cdaf1d2beef9/` 子目录。报告中的镜像身份一致；指定 token 的输入拒绝、当前同值无写入/不中断、完整 Basic/Bearer、换值撤销和重启持久化已有实现及回归证据。

当前升级仅支持部署手册登记的已发布静态 `[users]` 基准，不再提供未发布动态状态中间版本迁移。实际接入固定上述版本或后续完成同等独立验证的版本；历史开发镜像和 schema1 验证不作为当前交付依据。

本轮是源码与既有产物审查，未重跑构建、VM 或客户端测试，也未核验生产部署。GUI 文件操作、浏览器附件落盘仍未验证；Agent Space 本轮免验不免除新增 Admin/Android 界面的验收。历史 SDK `local shared arena connection is closed`/HTTP 502 重跑未复现，当前完整验证通过也不证明其底层根因已修复，集成传输验收仍保留失败诊断。

### 2.2 可以复用的 Agent Space 能力

| 能力 | 当前实现及限制 |
|---|---|
| 隔离与持久化 | 单节点、每账号独立 microsandbox VM、根盘和 `/workspace` 卷；同用户多设备/多会话共享该空间，不是每会话独立沙箱 |
| 状态权威 | `StateStore` 的 `state.json` 保存账号、凭据摘要、`spc_*` 和精确资源清单；单进程排他写入；缺失/损坏不自动初始化 |
| 管理 API | 创建、查询、启用/禁用、MCP key 轮换、DAV key 轮换/撤销、异步删除；单用户变更核对空间 ID；无独立 MCP key 撤销端点，禁用会撤销两类 key |
| MCP | `/u/{username}/mcp`；五个工具 `bash/read/write/edit/http_proxy`；请求及会话绑定身份，排队执行前复验；初始化和 tools/list 不启动 VM |
| 凭据 | 每账号一个有效 MCP key 和一个独立 DAV key；MCP 创建/轮换返回明文；DAV 支持调用方指定完整 token，省略仍随机生成，当前同值设置无副作用；GET 均不返回明文，禁用撤销两者 |
| 生命周期 | 禁用停止失败保留 `stopPending`；删除持久化意图后重试；未知 backend 状态不当作不存在、不重建磁盘 |
| 文件访问 | DAV 使用同一 `/workspace`、同一 SessionManager 和活动租约；支持 Range、条件请求、锁、常规文件方法 |
| 文件保护 | 完整 PUT 及普通文件 COPY 暂存后原子替换；普通文件到普通文件的 MOVE 不预删目标，但目录覆盖可能预删；目录递归可能部分完成；DAV 锁不约束任意 bash 写入 |
| 已有增强 | 修改时间、真实卷 used/available、路径修改预约；不支持部分 PUT、持久自定义属性或跨重启锁 |
| 域名边界 | MCP/admin、DAV、每空间临时网页按实际 Host/authority 隔离；不能只改返回 URL 或伪造转发头 |
| helper | guest 中由 Host 分发，具有实例归属、摘要及有界日志；禁止另建第二份 helper 或通配终止用户进程 |

### 2.3 集成方责任与实现对应

1. Core 已增加远程工作区管理、用户空间绑定、管理 API adapter、文件 API 和 Admin 界面；具体所有者见实现参考。
2. Relay 保留原共享 resource → route → upstream 分支，并增加显式 integration target、每用户绑定和在途撤销；发布、预算与 Usage 使用同一版本化归属。
3. Core 文件功能通过已有 DAV 协议实现。DAV 需由 Agent Space 部署方启用并配置合法独立 origin；Core 不远程改写其配置。
4. 现有管理 API 不提供持久服务实例身份、创建关联或条件修订。Core 不以新增这些字段为接入前提，异常按 7.3–7.4 核实和处理，不承诺所有故障自动恢复。
5. Agent Space 文档中的裸 usr 账号接入约定不是 username 校验限制。新 measix_ 映射由本 Core 方案定义，历史账号显式绑定原值；本次不修改 Agent Space 文档或代码。

### 2.4 关键实现核对位置

以下保留原调研定位；新增实现位置以实现参考为准：

| 所在仓库/文件 | 本轮确认内容与实施影响 |
|---|---|
| Agent Space `agent-space/agent-space-mcp/src/admin_api.rs` | v1 管理路由、创建输入及空间条件；当前 DavTokenBody 支持可选 token，拒绝 null/非法结构，Core 明确传值 |
| Agent Space `agent-space/src/state.rs`、`workspace_service.rs` | 禁用清两类凭据、stopPending、删除进度、每账号单个 DAV key；Core 按现有语义显示状态和处理凭据 |
| Agent Space `agent-space/agent-space-mcp/src/webdav.rs`、`agent-space/src/session.rs` | DAV 身份复验、流式代理及活动租约；Core 复用现有入口，不增加维护模式 |
| Agent Space `agent-space/webdav/src/lib.rs`、`fs.rs`、`tests/files.rs` | 根路径保护已实现；文件 ETag 来自 inode/长度/mtime/ctime 元数据，非内容快照 |
| Agent Space `agent-space/vendor/dav-server/src/handle_copymove.rs`、`conditional.rs` | COPY/MOVE 的源条件与目标 tagged If 不同；目录覆盖可预删目标，Core 首期限制为不覆盖 |
| Agent Space `agent-space/agent-space-mcp/src/origins.rs` | DAV 校验实际 authority 及浏览器来源；Hub 必须重建后端请求，不能照搬浏览器请求头 |
| Core `backend/internal/hub/capability/service.go`、`api/admin/admin.openapi.yaml` | RuntimeBinding 当前必须指向 Upstream；需增加显式目标分支 |
| Core `backend/internal/hub/upstream/service.go`、`backend/pkg/platformid/platformid.go` | CreateSecret 自开事务；新增工作区需要事务内复用秘密存储，并注册/校验新增标识 |
| Core `backend/internal/hub/security/token.go`、`secretbox.go` | 已有 RandomToken(32) 和加密存储；DAV 候选凭据先保存再设置，复用 SecretVersion，不新增派生算法/密钥 |
| Core `backend/internal/relay/control/store.go`、`backend/internal/hub/runtimecontrol/reconcile.go` | ACK、全局修订及恢复；工作区任务不能另设控制状态写入者 |
| Core `api/internal/usage-ingest.openapi.yaml`、`backend/internal/hub/budget/admission.go` | 准入/用量依赖 Upstream 归属及摘要，必须同步版本化扩展 |
| Core `backend/internal/hub/runtimecontrol/security_change.go`、`user_deletion.go` | Relay 确认后清理用户；远端清理目标必须在该事务删除用户数据前保存 |
| Android `app/src/main/java/net/weero/measix/pilot/` 下 `data/ai/mcp/McpConnectionDefinition.kt`、`data/enterprise/EnterpriseSessionController.kt`、`data/enterprise/PlatformControlClient.kt`、`service/ArtifactUseCase.kt` | 托管 MCP 使用原企业授权上下文；平台请求与附件导入须沿既有 Session/应用所有者扩展 |

## 3. 术语、权威与项目边界

“远程工作区”是产品名称，对应已有 Hosted Workspace / AgentSpace。第三方集成类型仍叫 Agent Space；`AgentSpacePolicy` 是控制定义，`AgentSpace` 是运行实例。文件、VM 状态和用户开通状态不进入共享 Managed Snapshot。

| 事实/行为 | 唯一所有者 |
|---|---|
| 企业身份、用户资格、管理员权限 | Control Hub |
| 集成配置、开通意图、凭据安全保存、操作恢复 | Control Hub |
| 企业 MCP 定义、Assistant 引用及 Release | Core 现有能力发布领域 |
| MCP 用户绑定的应用、鉴权和透明转发 | Runtime Relay |
| 远端账号、空间 ID、凭据有效性、VM/磁盘/文件 | Agent Space |
| 经过 Core 的文件准入、管理审计、Client/Admin 文件协议 | Control Hub |
| 用户 WebDAV 客户端的直接访问 | Agent Space 按 DAV 凭据鉴权，Core 不参与每次请求 |
| 文件原子修改、目录边界、guest 文件读取 | Agent Space 现有文件与运行所有者 |
| 文件预览渲染、选择器和传输交互 | Admin / Android |

Core 不读写 Agent Space 状态文件，不调用 microsandbox，不在宿主挂载运行中的用户盘，不维护另一份文件树或 VM 回收器。Agent Space 不识别 Core 的 Release、Assistant、Android Session、Realm 或管理员角色，不读取 Hub 数据库；`measix_` 对它只是合法账号字符。

Core 保存的是接入绑定和操作意图，不是第二份账号权威：显示名仍来自 Core，实际空间/磁盘存在性仍来自 Agent Space。服务故障时保留最近观测和时间，并标记未知，不把缓存当作当前运行事实。

Core 文件链路为 `Admin/Android → Hub 文件 API → Agent Space WebDAV`；用户也可使用 `外部 WebDAV 客户端 → Agent Space WebDAV`。MCP 保持 `Android → Relay → Agent Space`。S1 合同明确新增 Hub 文件流职责，模型/MCP Runtime 正文仍走 Relay。用户直连不经过 Core 的 Session、预算或访问审计，不承诺 Core 全渠道记录文件访问。

Admin 和未来 Android 的文件界面消费同一 Core 文件 API，底层均复用现有 DAV，不分别为两个客户端开发 Agent Space 文件接口。用户拿到 DAV 地址、用户名和完整 Token 后，可自行选择支持相应标准方法与 Basic/Bearer 认证的 WebDAV 客户端；不限制指定品牌。具体客户端的扩展功能和兼容性以实测为准，不把“开放标准协议”写成“所有软件所有功能均已验证”。

## 4. 账号、空间与绑定

### 4.1 新账号映射

```text
Core:        usr_550e8400-e29b-41d4-a716-446655440000
Agent Space: measix_550e8400-e29b-41d4-a716-446655440000
```

算法是严格验证 Core User ID 后，将固定 `usr_` 前缀替换为 `measix_`，UUID 不改变。结果 43 字符，满足当前 64 字符上限。生成只发生在新绑定创建时，之后从绑定读取 `remoteUsername`。

完整随机 UUID 满足正常独立部署的唯一性要求。前缀标记产品来源，不构成权限隔离；克隆同一 Core 数据库会保留同一用户身份，不能据此认为克隆部署拥有另一批空间。克隆、接管和服务迁移须显式处理，不能自动领养同名账号或按前缀批量删除。

### 4.2 绑定与接管

`workspaceServiceId = wss_<uuid>` 标识 Hub 的第三方连接配置，`workspaceOperationId = wop_<uuid>` 标识持久工作区操作，均按标识合同使用。`agentSpaceId` 由 Agent Space 创建；Core 保存 workspaceServiceId、remoteUsername、spc 和配置目标，不再生成平行空间 ID。

Core 的 platformid 已登记 wss/wop/spc，校验导入的 spc 格式；Core 不生成远端 spc。OpenAPI、数据库约束及 fixtures 使用同一标识合同。

configRevision 是 Hub 配置版本，bindingRevision 是 Hub 用户绑定/授权意图版本，controlRevision 属于既有 Relay 控制投影。它们都不是 Agent Space 的条件写入参数。现有单账号管理变更携带原 spc，可防止操作同名重建的空间，但不能阻止同一空间上的迟到写入。

现有裸 usr 或人工账号只能通过显式接管绑定：核对所连接部署、原 spc、目标企业用户和操作者，保存原 remoteUsername，不改名、不复制或重建文件。同一集成/空间不能绑定给两个 Core 用户；不能借新建另一配置重复接管同一远端账号。本地唯一约束不是跨 Core 部署的所有权锁。

接管须确认原控制方停止编排且旧管理请求已结束，保存接管意图，使用现有禁用 API 撤销旧 MCP/DAV key 并等待停止，再按原 spc 恢复、签发并保存 MCP 凭据。需要文件访问时由管理员显式签发并交付新的 DAV 连接信息。数据库克隆或独立服务迁移需显式移交；不按前缀自动认领或批量删除，不以相同 URL/账号证明控制权。

## 5. Admin 远程工作区管理与热更新

### 5.1 页面与配置

Admin 新增“远程工作区”（`/admin/remote-workspaces`），以启用开关和“保存配置”为主流程，保存后应用启用或关闭意图，另有已保存连接检查、用户空间、操作进度和诊断。用户详情复用同一后端操作。服务关闭时隐藏未开通用户的工作区页签及 MCP 新增入口；已有空间、待处理操作和现有 MCP 定义仍可查看、清理，不把保存的配置和文件隐藏丢失。

专用 WorkspaceService 结构包含 ID、类型、名称、配置版本、启用意图、生效状态和诊断。Agent Space 使用类型化配置，首期不引入动态插件加载、任意脚本、通用 JSON 编辑器或自动发现服务。

运行配置包含管理/MCP 地址、用于文件功能的 DAV origin、管理凭据版本引用及必要超时。DAV origin 必须符合远端部署配置，外部客户端使用可达的 DAV 地址；临时网页域名仍由 Agent Space 独立管理。DAV 未启用时显示文件不可用，不阻止独立满足条件的 MCP 使用。

管理员输入管理凭据后只返回 Secret 引用及版本；管理 Bearer 不回显。DAV key 仅在有权限管理员显式查看时展示，见 8.2。所有秘密均不进浏览器持久状态、URL、审计或日志。数据库配置是集成运行权威，不让环境变量覆盖 Admin 修改。主密钥、监听地址、数据库路径及服务自身的 TLS/DNS 仍为部署配置；“热更新集成”不表示远程改写 Agent Space 的 config.toml。

### 5.2 应用协议

1. 更新携带 expectedRevision；先验证字段并保存候选版本，重复提交使用相同幂等键。
2. 后台按支持版本清单检查配置地址、管理认证及已有 API 响应；已有绑定核对原账号/spc。只读检查不创建账号、不签发凭据、不启动 VM。没有账号或尚未完成 DAV 请求时，文件能力显示待验证；真实文件访问可能启动 VM，不伪装成无副作用连接检查。
3. 已有该集成的生效运行绑定时，将新目标编译进现有 Hub→Relay 控制投影，以 controlRevision 和 bundleHash 应用；不增加 managedGeneration。
4. Relay 原子应用并返回精确确认；Hub 在持久化确认后更新 activeConfigRevision，并开放与该配置一致的文件准入。
5. Hub 重启或确认丢失时查询已应用版本收敛，不盲目重发旧配置覆盖新状态。

尚无 Release 或集成尚未发布/无运行绑定时，连接检查和配置应用在 Hub 内以数据库事务生效，标记“MCP 待发布/无运行绑定”，不依赖现有要求 activeReleaseContent 的 Upstream 应用入口，不伪造 generation=0 的发布。首个关联 Release 仍由现有 Publish 应用 Relay 控制；发布之前即可开通新用户工作区和管理文件，MCP 为可选后续步骤。已有工作区在撤下 MCP 后仍可进行文件和生命周期管理。

跨 Hub/Relay 切换不宣称分布式原子事务。发送控制前持久化目标及待应用状态；发生目标、凭据或撤销语义变化时先关闭相关 Hub 文件准入，再应用 Relay，确认后重开。只影响后续请求的普通超时参数变更可让已准入请求按旧配置完成。Relay 重启当前状态为空、拒绝运行请求，Hub reconcile 从持久权威重新下发；ACK 不是 Relay 落盘、远端身份有效或 VM 停止的证明。

复用 `upstream` 的 Secret/配置版本机制和 `runtimecontrol` 的 Activation、幂等与 reconcile 模式，但业务操作留在集成/工作区领域，不把创建 VM 塞进 Upstream 服务。全局控制修订仍由现有唯一写入路径串行分配，不能由每个用户工作流各自覆盖整份 Relay 状态。

远端创建、等待停机或删除轮询不长期占用全局配置 Activation；只有编译/应用控制修订的阶段进入现有串行协议。空间任务先保存自身进度，控制应用忙时等待重试，避免一个离线工作区阻塞所有无关配置更新。真正已发送且结果未知的控制应用仍须先对账，不能为绕过互斥另造控制写入口。

配置校验失败且尚未发送应用时，可确定旧版本继续有效；发送后结果未知时必须标记 APPLYING/UNKNOWN 并对账，不能报告已回滚。新凭据如果已在远端轮换，旧凭据可能已无效，也不能承诺透明退回旧值。

已有绑定时，更换地址必须由管理员确认仍指向原部署，并逐一核对原账号/spc；无法核实则不应用。迁移到另一服务走显式移交流程，不把本地 workspaceServiceId、TLS 或相同 spc 当成远端实例身份证明。使用合法 authority/TLS 名称，不靠伪造 X-Forwarded-Host 绕过校验。远端返回的 mcpUrl/davUrl 必须与已配置 origin 和目标账号匹配，不作为任意带凭据请求或重定向的来源。

### 5.3 生效与停用

配置状态、连接健康、MCP 是否已发布分别展示。有效配置不等于远端此刻健康；单个集成故障不停止其他 Core 能力。

普通参数切换：新请求使用新版本，在途工作保留原目标。安全变更：关闭受影响请求准入，取消关联连接；用户 Token 轮换会使旧 MCP session 失效，应重新初始化而非重放 tools/call。

停用集成先持久化停用意图，阻断 Hub 文件请求，向 Relay 撤销运行绑定，同时协调停止本集成登记的远端空间。完整停用以 Relay 撤销及远端停用均确认作为完成条件；不可达时显示进行中和哪一侧未确认，不承诺网络分区下瞬时撤销。管理收尾可继续使用保管的管理凭据。Core 本地阻断不影响用户直连 DAV；必须确认远端禁用后才能宣称这条路径也已撤销。

重新启用只恢复仍期望连接且企业身份有效的绑定，不能恢复用户主动断开或待删除空间。集成关闭保留配置、秘密引用和清理记录；只有相关空间清理/显式移交完毕、MCP 引用解除后才能删除集成记录，避免遗留无人管理的磁盘。

## 6. MCP 发布与每用户运行绑定

### 6.1 一个企业 MCP 定义

每个集成生成一个稳定 `mcpServerId`，展示名“远程工作区”，协议 MCP_STREAMABLE_HTTP，authOwnership 保持 ENTERPRISE_MANAGED。它不是 USER_MANAGED OAuth，也不使用管理员 Bearer 执行用户工具。

保存服务配置不修改能力草稿。管理员需要助手工具时，在 MCP 页面显式添加该服务的稳定定义及内部运行关联，再经过 Validate/Publish 启用工具入口；用户开通和文件管理不依赖此步骤。Admin 明确显示“MCP 未发布（可选）”。不要建立旁路发布，也不要为每个用户复制 MCP 定义、Assistant 或 Release。

集成维护连接和凭据；公开名称/Assistant 引用按现有能力发布权限管理。用户开通/断开和明确操作触发的凭据轮换只更新用户绑定与 controlRevision，不改变共享 Snapshot。配置更新不改变 mcpServerId；替换独立服务须显式处理。

未开通用户可能仍在共享企业目录看到该 MCP：新 Android 结合工作区投影显示“未开通”，Relay 始终拒绝执行。不得因共享目录可见推断已授权。需要工作区的 Assistant 未获得可调用资源时报告明确准备失败；不静默跳过固定引用或回退本地 shell。未引用该服务的普通 Assistant 不受工作区不可用影响。

当前 Admin `RuntimeBindingDefinition` 强制要求 upstreamId，能力校验和发布编译也直接查共享 Upstream，因此这里需要同步扩展服务器端发布合同，不能仅在 Relay 加一张表。增加显式 REMOTE_WORKSPACE 目标分支，引用 workspaceServiceId，与 UPSTREAM 分支互斥，首期仅允许绑定该集成的 MCP 资源；历史缺少判别的绑定按原 upstreamId 解释，不重写历史 Release 字节/hash。新集成定义按受支持接口、已配置目标和 MCP 路径校验。不要创建一个 Auth=NONE 的假 Upstream 或放入管理员 Token 来绕过校验。新 wire 版本和对应读取/发布 fixtures 须在 P0 固定；客户端 McpDefinition 无需暴露此分支。发布回滚和工作区 reconcile 均要以当前有效 Release 为准，不得复活已从发布中撤下的资源。

该绑定分支是必需的服务端合同扩展，不能在保持 upstreamId 必填的旧 DTO 中塞入 workspaceServiceId。旧 Upstream 分支保持原字段含义；新分支有显式类型判别和协议支持检查，并覆盖草稿编辑、校验、发布、回滚、控制重建与历史读取。不得为了让旧服务端接受而生成占位 Upstream ID。

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

现有 `BudgetAdmissionRequest`、`RequestUsageFact` 和准入校验要求合法 upstreamId；发布与 Relay 改为集成目标后，这条链路也必须同步扩展。选择类型化目标归属：保留历史 Upstream 分支，新分支保存 workspaceServiceId 和固定的用户空间/绑定身份；协议版本、确切字段及 Admin 投影在 P0 一起冻结。不把 workspaceServiceId 填进 upstreamId，不造虚构 Upstream，不跳过原有 MCP 预算准入。

准入摘要、预算请求持久化、Relay spool、Usage ingest/ledger、请求详情及对账都必须保存请求发生时的不可变目标，不能在集成改名、用户删除或空间重建后查询当前绑定补历史事实。旧 spool/账本按原合同读取，追加迁移保留历史归属；新事件不下发给不支持该分支的消费者。用户未开通等权限拒绝仍由授权层拒绝，不能套用计量降级策略放行。

Direct MCP 计量仍记录转发请求，不等于 JSON-RPC 工具调用次数、VM 时间或存储用量。经过 Hub 的文件传输记录操作/字节审计，外部 DAV 直连不进入 Core 请求账本，均不伪装成 MCP 用量；compute/storage 由 S1 相应合同后续定义。

## 7. 用户生命周期、状态与恢复

### 7.1 状态模型

持久化期望状态采用连接、断开、删除三种意图；当前进度由待完成操作、Relay 确认和远端观测派生，避免多组可独立修改的 enabled/connected/ready 标志。

UI 至少区分未开通、开通中、已连接、断开中、已断开、恢复中、删除中和需要处理的错误。VM 休眠是“已连接”的运行状态，不显示为断开；UNKNOWN 不当作未开通。

连接意图、访问可用性和 VM 状态分开投影。客户端不得只用一个 connected 布尔值同时决定文件和 MCP 权限：

| 条件 | Core 用户文件 | 用户 MCP | Core Admin 文件 |
|---|---|---|---|
| 集成生效、用户有效、空间连接收敛，MCP 已发布 | 可用 | 绑定已应用且 Assistant 引用时可用 | 可用 |
| 上述空间保留连接，但 MCP 从 Release 撤下 | 可用 | 不可用，撤销 Relay 绑定 | 可用 |
| 空间已断开且停机已收敛，集成生效 | 不可用 | 不可用 | 不可用 |
| 企业用户被禁用且远端停机已收敛，集成生效 | 不可用 | 不可用 | 不可用 |
| 空间连接/MCP 正常，但 DAV 关闭、无 key 或 key 失效 | 不可用 | 按原 MCP 条件可用 | 不可用 |
| 停机待完成、删除中或目标身份未确认 | 不可用 | 不可用 | 不可用，仅诊断/收尾 |
| 集成停用或目标服务不可用 | 不可用 | 不可用 | 不可用，仅诊断/收尾 |

撤下/重新发布 MCP 只改变该资源的运行准入，不隐式断开账号、删除文件或轮换凭据；重新发布仍要求当前用户资格和连接意图。表中的文件可用还要求远端 DAV 已开启、账号 active 且 Core 持有有效 DAV key。用户外部 DAV 客户端只受远端账号和 DAV 凭据约束；Core 本地拒绝与远端撤销必须分别显示。

### 7.2 操作次序

| 操作 | 必须执行与完成条件 |
|---|---|
| 创建并连接 | 检查服务已启用及用户资格 → 保存创建意图 → 创建远端账号 → 保存 spc/加密 MCP 凭据 → 如有已发布 MCP 则应用用户绑定并确认生效；文件凭据按 8.2 准备，分别显示可用性；创建不启动 VM |
| 断开 | 保存断开意图、立即阻断 Hub 用户文件请求 → 撤销 Relay、禁用远端账号并停止 VM → 双侧确认；保留文件 |
| 恢复 | 复验用户资格、配置目标/原 spc 且停止完成 → 恢复账号 → 签发并保存 MCP Token → 应用绑定；DAV 保持不可用，由管理员显式重新签发并交付 |
| 删除空间 | 保存原 spc 的删除意图并撤销访问 → 远端 DELETE → 查询原目标至确认不存在 → 清除秘密和活动绑定 |
| 企业用户禁用 | 立即沿既有身份链撤销平台访问，再协调远端禁用；保留原连接意图以区分管理员断开 |
| 企业用户恢复 | 仅恢复原有连接意图且集成仍生效的空间；按上述恢复流程准备 MCP，DAV 仍需显式签发，不复活删除任务 |
| 企业用户删除 | 先拒绝身份访问，再以最小独立清理任务完成远端删除；不因级联删除用户行丢失清理目标 |
| 单设备退出/撤销 | 关闭经过 Core 的该设备/Session 请求，保留空间及其他设备资格；不撤销全账号 DAV key，不影响外部 DAV 客户端 |

创建、断开、恢复、删除使用 Admin 权限和幂等命令。首期 Android 不提供服务级开通/断开开关；未来自助开通需单独定义授权，不能从 MCP 可用性推导。

### 7.3 创建和凭据的不确定结果

Core 在调用前保存操作目标和进度，Admin 命令使用本地幂等键；本地去重不意味着远端 API 幂等。每个管理写步骤在发送前持久标记“可能已发送”；成功后结果与绑定变更在同一 Hub 事务确认，提交后才能开放访问。创建/MCP 轮换须同时保存返回的 spc/加密 MCP Secret 引用；DAV 则先保存候选 Secret 和操作引用，再设置远端，见 8.2。当前 CreateSecret/ReplaceSecret 自开事务，应复用既有 SecretBox/SecretVersion 的事务内写入，不把秘密与操作分两次提交、不另建秘密存储。提交失败或提交结果未知时先核对本地操作记录，不能直接重发远端写入。创建 409 或响应丢失时先 GET：同名账号存在只能证明存在，不能证明属于该次创建。没有可信归属依据时进入管理员核实/显式接管，不自动认领、轮换或删除。

创建结果未知时的取消仍保留原目标和诊断。一次 GET 404 不能证明旧 POST 不会到达；不自动重发创建来“收敛”，不把未知创建或清理标记完成。管理员须结合远端实际状态和在途请求情况核实；无法核实继续待处理。

MCP Token 明文只在签发或轮换时返回，响应到达后先加密保存；保存失败或响应丢失，保持 MCP 不可用，查询不会返回原明文。待目标及前次操作核实后由管理员显式重新签发，不在后台循环反复轮换。DAV Token 由 Core 预先生成并保存，设置响应丢失不再导致原值丢失；按 8.2 沿同一值处理，不因此生成另一个 Token。轮换前关闭相关 Core 准入并取消旧连接；DAV 换值使直连客户端旧 key 失效，提示重新配置。未解决的结果未知不能仅因后来读到账号 active 就恢复访问。

MCP 凭据由 Core 管理用于 Relay。DAV 凭据由 Core 保管并可由管理员交付用户；外部 Agent Space 管理员仍能独立撤销或轮换。Core 发现失效时显示待核实，不与外部管理方自动轮换竞争。服务级管理 Bearer 不提供多管理方的细粒度隔离，账号前缀也不提供这种隔离。

### 7.4 删除、重启与并发

同一账号管理操作串行执行，固定配置目标、原 spc 和本地绑定意图版本；旧 Core 任务不能发布过期绑定。远端写入只使用现有 API 的 agentSpaceId 校验，不发送不存在的条件修订。若管理写入结果未知，继续必要查询，但不自动发出与其冲突的启用、轮换或重建；不可确认时显示待处理。Core 本地串行不能撤回已发出的请求，也不承诺彻底拒绝远端迟到写入或删除后旧创建重放。

远端已确认接受的禁用/删除由其现有机制继续；Core 查询 stopPending/deleting 和清理结果。原目标的删除 204，或经正确路由/管理认证且没有未解决创建的 GET 404，可作为完成依据；网关 404、不可达或另一部署的响应不能作为清理完成。空间 ID 失配时停止旧操作，不能用新 spc 重试旧删除。确认删除并解决前次未知操作后才能重建；新 spc 使旧会话和文件引用失效。

未完成操作保存在 Hub，复用既有后台循环和有界退避，恢复已确认、可继续的步骤及进度查询；不引入独立队列、通用任务平台或每用户常驻定时器。人工核实和重试沿同一操作记录保留诊断。文件修改和 bash 的未知结果不自动重放。

| 持久步骤状态 | 重启/人工处理规则 |
|---|---|
| 尚未标记发送 | 复验当前意图后可以执行；意图已取消则不发送 |
| 可能已发送、未持久确认 | 保守保留待确认，不当作未发送；包括标记提交后、实际发送前崩溃。普通写入只查询/核实，DAV 指定值设置按 8.2 的同值恢复约束处理 |
| 结果及绑定/秘密已持久确认 | 按已确认结果继续下一步；返回的账号/spc 与固定目标不符则停止，不发布凭据 |
| 收到结果时本地意图已改变 | 保存必要结果用于原目标收尾，不重新开放访问；旧结果不能覆盖新意图 |

Admin 的处理入口沿原操作提供“重新查询”“显式接管”“核实后重试/继续”，记录核实依据、操作者和固定目标；不提供跳过核实的“标记成功”。涉及控制权变更或不同值写入的接管/重试前，由部署管理方确认旧控制方及在途写入已结束；DAV 同值继续按 8.2 处理。必要时按 Agent Space 现有运维方式处理，Core 不自动重启服务。不能仅凭账号 active、一次 404 或经过一段时间解除未知状态，证据不足继续待处理。

操作固定实际配置修订、地址及管理 Secret 版本，不能只保存 workspaceServiceId 后改读最新配置；有未确认写入时禁止迁移目标。地址迁移须在处理完这些操作后核对原空间，再显式更新后续操作目标。管理凭据失效时允许管理员在核实同一目标后显式更新待处理操作的凭据引用并留下记录，不因此换目标或重发结果未知的写入。

企业用户删除须接入当前 finalizeSecurityChange → purgeUserData：在同一 Hub 事务先保存独立远端清理记录，再清除用户私有数据。记录仅保留目标配置、账号、原 spc、未完成操作及必要秘密引用，不保存文件树或正文；创建未获 spc 时也必须保留未知创建目标。用户行删除后仍能收尾，不能把 Core 身份删除完成解释为远端磁盘删除完成。

远端不可达不长期占住全局 Activation。清理未完成前保留必要集成/秘密和 Admin 进度；只有远端禁用/删除确认后才表示用户直连访问也已撤销。既有身份删除的 deny-first、凭据墓碑和预算收尾保持原义，不为远端增加新协议。

## 8. 使用现有 Agent Space 接口

### 8.1 接入条件和 adapter

本次 Core 集成不修改 Agent Space 项目，复用其独立实现的指定 DAV token 能力，不再新增 info、服务实例身份、创建去重/墓碑、账号修订、管理文件路由或维护模式，也不另行修改 StateStore、Session、helper 或磁盘 schema。Core adapter 使用 /admin/v1/status、用户管理/凭据 API、每用户 MCP 和 DAV；这些能力由 Agent Space 自己的接口文档拥有，Core 不复制一套远端 OpenAPI。

远端需使用支持动态管理 API 的版本；本方案的文件接入还要求包含指定 DAV token 合同、开启 WebDAV、配置合法独立 origin，并提供 Core 与外部客户端所需的网络可达性。采用 Agent Space 自身完成发布验证的对应版本，不再提出 Core 专用改造。旧 API 可能忽略未知 token 字段后随机生成，因此须按受支持版本清单接入；status 成功不证明支持指定 token，不能在真实账号上以省略字段或静默降级探测。现有 status 不返回版本、服务实例身份或 DAV 能力清单；部署方核对镜像/发布身份，Core 记录该接入基线及地址，不把版本检查实现成读取不存在的字段。连接检查与实际 MCP/文件闭环分别报告。

Core 管理请求带服务级 Bearer，单用户变更固定原 spc；MCP 使用 MCP key，文件请求使用独立 DAV key。Core 不用管理员 Bearer 读写文件，不调用 bash 模拟文件 API。WebDAV adapter 负责标准方法、结构化 XML/多状态结果解析及路径、Destination、条件头映射；不做字符串替换，不把任意外部 URL 转发给远端。

### 8.2 DAV 凭据与管理员交付

Core 生成并保管 DAV Token，Agent Space 接受设置并拥有其实际有效性。复用 `security.RandomToken(32)`：32 个密码学随机字节经 Base64URL 无填充编码得到 43 字符，满足当前 32–256 字符输入要求；不使用用户名/UUID、登录密码、Session、管理 Bearer 或 MCP Token，不新增派生密钥或 DAV 版本计数。每工作区复用一个 Secret 及其不可变 SecretVersion；绑定指向已确认有效版本，待完成操作指向候选版本，不用“最新版本”隐式替换有效凭据。

首次设置和管理员重设共用一个流程：

1. 核对账号 active、原 spc、集成和用户资格，持久化操作；重设先关闭 Core 文件准入并取消旧传输。
2. 生成新 Token，在同一 Hub 事务保存加密 Secret 版本及本次操作的候选引用；保存失败不发送远端请求。数据库中只保存密文，Token 不进日志、URL 或浏览器持久状态。
3. 持久标记发送步骤，调用 `POST /admin/v1/users/{username}/dav-token`，JSON 明确包含原 `agentSpaceId` 和已保存的 `token`，服务级管理 Bearer 仅由 Hub 使用。
4. 核对成功响应的账号、spc、DAV 地址及 token 与固定目标/提交值一致，在同一事务确认操作并切换绑定的有效 Secret 版本；当前意图已改变时只保存收尾结果，不重新开放访问。成功后才允许文件代理和管理员查看该值。

指定 token 非法返回 400，已被其他 DAV 账号或当前 MCP 凭据使用返回 409；保留明确失败，不省略 token 重试、不回退随机签发、不自动采用不匹配的返回值。响应丢失或确认事务失败时保留同一候选值并核对本地记录，不再生成新值。只有同一目标、同一操作且授权意图仍有效、没有后续撤销/重设或外部控制变更时，才可按原值继续设置；当前同值请求不换 keyId、不写盘、不取消传输。这不保证跨轮换的旧请求幂等：后续改值/撤销完成前仍须解决旧在途请求，无法确认则待处理，不后台反复覆盖外部变更。

用户查询中的 `hasDavCredential` 只表示远端存在某个 DAV 凭据，不返回 keyId、摘要或明文，不能证明 Core 保存的候选/有效值仍匹配，也不能据此确认未知设置成功。实际 DAV 请求才能检验所提交凭据的访问能力，且可能启动 VM；它仍不能证明旧管理请求已结束。无明确失效证据时显示最近确认结果及观测时间，不能声称实时有效。

新账号开通且 DAV 已启用时，在账号 active 后执行上述首次设置；失败只影响文件，不把可用 MCP 误报为失败。已有账号缺少 key 或操作失败时由管理员显式处理，不由页面访问触发签发。Admin 用户工作区提供“WebDAV 连接信息”：DAV 地址、用户名、默认遮蔽的 Token，以及受控查看/复制、签发/重设、撤销。管理员可交付用户，用户在标准客户端中以用户名和完整 Token 作为 Basic 凭据，或以完整 Token 作为 Bearer；自定义 Token 不从返回的 keyId 拼接。首期不做用户自助领取、审批、限时凭据或多凭据体系。

查看仅解密目标工作区当前已确认的有效版本，不接受任意 secretId，返回前复验管理员资格、绑定及 Secret 版本。响应 no-store，关闭页面后清除展示；审计只记录操作者和动作，不记录秘密。该入口不开放通用 Secret 明文读取，也不展示管理 Bearer 或 MCP key。未保存的历史 key 不能从 Agent Space 取回，须显式重设；查看不能暗中调用轮换。候选/结果未知、禁用或撤销期间不将旧值展示为有效凭据。

每账号只有一个有效 DAV Token，Core 文件代理与用户直连共享此值；重设成功后旧值失效，用户需更新外部客户端。撤销先阻断 Core 文件访问并取消旧传输，再调用远端 DELETE，确认后清除有效引用；只有远端确认且旧设置请求已收敛，才表示直连也已撤销，不影响 MCP。禁用/删除同样撤销两类凭据。恢复操作只自动准备 MCP；需要文件访问时，由管理员显式按上述流程生成新 DAV Token、设置并重新交付，不重新启用历史值。企业用户恢复和集成重新启用也遵循此规则。发现远端管理员独立换值时标记失效/待核实，不自动竞争设置；后台查询或页面打开不重新授权。

### 8.3 DAV 文件边界

普通账号文件访问要求 active、DAV 已开启且凭据有效；已禁用、stopPending 或 deleting 不开放 Core 文件入口。没有管理员维护例外，不临时 PATCH active 来读取禁用账号文件。DAV 关闭时不通过管理接口或 helper 旁路访问。

Core 和用户直连访问同一 /workspace，复用远端现有根保护、原子写入和活动租约。根目录及等价别名禁止删除/移动/覆盖；拒绝穿越、symlink、特殊文件和保留目录。保留 Range、条件头及原子 PUT/普通文件 COPY；普通文件到普通文件的 MOVE 不预删目标，但现有目录覆盖可先删除目标。Core 首期目录 COPY/MOVE 只允许目标不存在，不提供目录合并/覆盖。目录递归可能部分完成，Core 不把 DAV 207 当全部成功。DAV 锁不约束任意 bash 写入，外部客户端也可能同时修改文件；Core 的首期 UI 限制不改变独立 DAV 客户端可用的方法。

Core 仅对自己的文件请求提供企业鉴权、Session 取消和操作审计。直连客户端按远端账号/key 鉴权，不受 Core 登出或单设备撤销控制，也不进入 Core 文件审计/预算。需要撤销所有文件访问时执行远端 DAV key 撤销；需要停止工作区时执行远端禁用。远端未确认前显示未完成，不声称本地取消已撤销所有直连访问。

## 9. Core 文件 API、权限与预览

### 9.1 两个入口，一个应用服务

Admin 入口验证管理员角色、Cookie Session、修改动作 CSRF 及目标用户；Client 入口验证原企业 Session，用户身份从认证主体获取，不接受其他 userId。两者共享同一文件应用服务和 Agent Space adapter。

每次 Core 文件操作固定集成配置、用户、原账号/spc、DAV Secret 版本、原授权上下文及本地绑定修订。检查集成生效、用户资格、连接意图和目标状态；转发前复核。配置/身份/绑定撤销取消在途请求；切域、登出、重建后旧结果不能写入新视图或新文件缓存。

文件请求登记与撤销使用与 6.2 相同的登记后复验原则；Admin 登出、会话失效和权限收回也取消以该 Admin Session 发起的传输。Hub 发往远端使用用户 DAV key，必须持续约束本次转发的企业主体及本地绑定版本；远端按其已有账号/key 复验，不接收 Core Session 或本地修订。

Core 用户和管理员均不能访问断开、禁用或删除中的空间；关闭整个集成后只保留诊断和收尾。管理员文件权限覆盖内容访问，UI 明确显示目标用户和空间，审计区分操作者与资源所有者；不增加维护入口。

文件审计记录 actor、目标用户/spc、动作、经规范化的目标路径、开始/终态、字节数及请求关联 ID；记录谁读过/下载过文件，不记录文件正文、Token 或管理凭据。审计路径也属于受限企业数据，仅向有权限的管理员展示；不借日志保留删除用户的完整私有文件索引。关联信息留在 Core，不要求 Agent Space 扩展企业审计。用户直连不经过此审计链。

### 9.2 文件操作与传输

Core 使用类型化 JSON 元信息/操作请求，以及流式上传下载正文；具体 OpenAPI 先于代码。公开路径使用工作区根内相对路径，空路径只表示根目录；源/目标都经过同一验证。标识和路径不是授权证明，任何 URL/query 都不包含凭据。

| 操作 | 关键约束 |
|---|---|
| 列目录/元信息 | PROPFIND 使用显式属性及 Depth: 1/0；限制 XML 字节/条目并解析 response/propstat 状态；不支持分页、全树索引或增量同步，超限提示细分路径 |
| 上传 | PUT 原始流；新建使用 If-None-Match: *，显式替换使用目标 ETag 的 If-Match；不提供无条件覆盖 |
| 下载 | GET/HEAD，保留必要 Range/条件响应及文件名；每次先鉴权再处理 304；不得向第三方跳转携带凭据 |
| 新建目录 | MKCOL；同名/父目录不存在等冲突有稳定错误 |
| 重命名/移动/复制 | MOVE/COPY；固定同一空间的 Destination，默认显式 Overwrite: F；仅普通文件可按目标 ETag 显式覆盖，目录只允许目标不存在 |
| 删除 | DELETE；根不可删，文件按 ETag 条件删除；目录确认递归范围并报告部分失败，结果未知先查询，不自动重放 |

元信息至少包含相对路径、条目类型、文件长度、修改时间及可用 ETag；目录容量使用服务实际提供的卷 used/available，不将其显示为 Core 用户 quota。元信息缺失时明确缺省，不能合成假的创建时间或文件摘要。修改请求显式区分“不覆盖”“按指定版本覆盖”；条件值由文件元信息获得，使用现有 DAV 条件头；Core 本地绑定修订不传作远端文件条件。

COPY/MOVE 的 HTTP If-Match 检查源资源，不能用它表达目标未变化。普通文件显式覆盖使用 Overwrite: T 和唯一一条以固定目标 URL 标记、含目标 ETag 的 DAV If 列表；需要固定源版本时另用源 If-Match。不要用多条 tagged If 列表冒充同时成立的条件，当前解析按列表择一匹配。adapter 从类型化路径/ETag 构造这些头，不接受客户端原始 Destination/If；缺少目标 ETag 则不能条件覆盖。禁止“HEAD 后无条件写入”模拟条件更新；条件冲突返回冲突供用户刷新确认，不自动去掉条件重试。目录树修改没有整体事务或快照保证。

Hub 新建后端请求，只映射经过校验的文件头并注入目标账号 DAV Bearer；不透传浏览器 Cookie、Authorization、Origin、Sec-Fetch-*、Host 或 Forwarded/X-Forwarded-*。请求使用配置的真实 DAV authority/TLS，不伪造浏览器 Origin。管理、MCP 和 DAV 的带凭据请求均禁止自动跟随重定向。响应只输出合同允许的元信息/文件头，不透传远端 Set-Cookie、WWW-Authenticate 或任意 Location；XML 禁用外部实体，href 解析后必须仍属于原账号路径。

远端 401/403 分类为工作区凭据/权限不可用，不能原样冒充 Core Session 失效而触发用户登出；DAV 路由 404、文件不存在、条件失败、锁冲突、容量不足和远端故障分别映射稳定 Problem。非根路径返回 404 时，按同一请求身份/绑定复验工作区根的 Depth: 0 PROPFIND；根正常才报告文件不存在，否则报告文件服务不可用/未知。根的 404 不解释为空目录或空间删除。实际 Core 认证失败仍按现有身份错误处理。

操作结果统一区分未执行的拒绝、成功、递归部分成功和提交结果未知。部分成功返回有界的失败路径/原因及是否截断，不能用单一 success 或 207 状态掩盖子项失败。传输中断后的文件提交可能已发生；客户端先刷新目标元信息，不能仅因取消便提示“未保存”。原子 PUT/COPY 只保证失败不留下半个替换文件，不保证已经提交的操作可撤销。

Hub 不整文件缓冲、不先落盘，使用背压和主动取消；不得对文件 PUT 沿用预览的 1 MiB 正文限制或 bash 的总执行超时。远端仅 PUT 放开正文大小，其他方法的控制正文仍上限 1 MiB；Core 控制正文限制不得高于此值。分别限制传输并发、连接/启动时间和无进展 IO 时间，并与远端配置协调；长时间没有请求/响应字节进展的服务端复制等操作也可能超时，按结果未知处理，不自动重放。Core 上限由实施仓库按真实测试确定并纳入交付配置，超限要有稳定响应；不靠任意增大缓冲解决大文件问题。

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

现有 ETag 是文件元数据校验值，不是不可变内容快照；DAV 锁也不限制 bash 改写文件。Range/条件请求只能发现可检测的变化，不能承诺并发 bash 或其他 DAV 客户端写入时获得一致版本。受控阅读器可完成一次有界下载后阅读该副本，这只保证后续查看不再随远端变化，不能证明下载期间没有并发改写。发现文件变化时提示等待写入结束再重试；超过客户端资源上限时提供下载，不为首期新增远端快照系统。

预览缓存按企业主体、spc、路径和文件版本隔离，使用 no-store 等适当响应约束；关闭阅读器撤销 blob URL、取消请求并释放临时资源。已下载或已展示的字节不能被远程撤销。Core 访问撤销阻断其后续读取并取消其在途传输，不控制外部 DAV 客户端，也不承诺收回已交付内容。

文件预览不调用 `http_proxy`、不启动静态网页发布、不生成匿名 URL。Agent 自行发布应用仍走现有独立预览 origin，不与 Admin Cookie origin 合并。Agent Space 不新增 PDF/Markdown 转码、缩略图任务或另一份文件副本。

## 10. Android 企业能力适配

分两步交付，但接口第一步按最终授权边界设计：

1. 工作区识别：查询本用户远程工作区状态、spc、对应 mcpServerId、文件能力与不可用原因；展示企业托管来源。继续由现有 Enterprise/MCP owner 创建连接，不写入个人 Settings，不保存远端 Token。
2. 文件界面：企业域内目录导航、上传下载、基本操作与四类预览；文件选择器上传和下载到用户选定位置；进度、取消、失败、冲突和存储不足可见。

Android 文件管理通过 Core Client API 复用与 Admin 相同的 Hub→WebDAV 链路，无需 Agent Space 新增 Android 接口，原生企业界面不保存 DAV Token。用户也可以另外在 Android 或其他平台的标准 WebDAV 客户端中，手动配置管理员交付的凭据直连；这是 8.3 所述独立访问路径。

UI 名称为“远程工作区 → 文件”。个人域不注册企业 MCP 或展示企业文件。切企业/退出/Session 撤销沿既有 owner 取消传输和阅读任务；异步结果始终核对原域、原主体和原 spc，不从当前页面反推授权。

文件 HTTP 调用沿 PlatformControlClient 和 EnterpriseSessionController 的原 Session 操作租约扩展，发起时捕获原身份并登记取消，不从全局“当前企业”临时取 Token；文件正文使用流式方法，不进入现有控制 JSON 的整正文解码。托管 MCP 继续使用 ManagedPlatform 定义、既有连接工厂和 generation/interaction 头，不创建第二套 SDK client。瞬时工作区不可用只影响状态/准入，不删除已有 MCP 工具目录或把连接失败写成永久未支持。

文件进入聊天附件时通过现有 ArtifactUseCase/ArtifactDraftScope 应用入口及原会话目标导入，UI 不直接写 ArtifactStore/DAO；远程路径不冒充本地已存在文件。手机文件必须上传确认后才能作为 `/workspace/...` 给远端 Agent；MCP 工具与本地 workspace 工具保留明确的执行位置，不因接入服务静默切换本地 shell 或同步 Skills。

工作区状态缓存只用于显示；离线不允许使用陈旧授权访问。Room/DataStore 如有新字段必须显式迁移；不清空企业聊天、个人文件或既有 MCP 目录。Android 的工具定义冻结和执行时授权复验继续遵守现有 Turn/MCP 协议。

## 11. 协议面与兼容策略

### 11.1 接口范围（确切路径以 OpenAPI 为准）

以下为当前接口职责索引；确切方法、枚举、错误和 schema 由 OpenAPI 维护，不在本文复制完整定义，也不在前后端手写平行 DTO。

| 接口面 | 当前职责 |
|---|---|
| Core Admin `/api/admin/v1/remote-workspace/services` | 集成创建/查询/更新、check、apply、disable，预期版本和幂等操作引用 |
| Core Admin 用户工作区子资源 | 开通、断开、恢复、删除、状态/进度、显式接管旧空间 |
| Core Admin 工作区文件/连接信息子资源 | 文件列表/内容与修改操作；受控查看/复制 DAV 连接信息及签发、轮换、撤销 |
| Core Client `/api/client/v1/workspace` | 单一用户工作区投影及其文件子资源，身份从 Session 获取 |
| Core 能力发布管理面 | RuntimeBinding 的 Upstream/集成目标互斥分支及历史 Draft/Release 支持 |
| Hub→Relay 内部控制 | 显式支持按用户的运行绑定、凭据引用/值及修订，应用/撤销确认 |
| Relay→Hub 用量/预算面 | 集成目标的准入、归属、摘要和用量事实；旧事件读取及 spool 重放兼容 |
| Agent Space | 无 Core 专用接口；采用支持指定 DAV token 且完成独立发布验证的管理 API，以及现有 MCP/DAV |

工作区投影含独立 schemaVersion、当前绑定状态、spc（未创建时缺省）、已发布 mcpServerId、分别可判定的 MCP/文件可用性及原因、文件能力、状态修订和观测时间；不返回管理员/MCP/DAV Token、真实私有上游地址或磁盘名称；Admin 显式查看 DAV 连接信息是单独受控接口，不进入 Client 投影。它只引用 MCP 定义，不拥有第二份工具 schema/配置。字段缺省与 null 的语义、未开通/集成关闭/远端未知示例须进入共享 fixtures。

管理命令返回可查询的持久操作引用；HTTP 202 只表示意图已接受，客户端轮询该操作和当前投影，不以请求完成时间推导生命周期完成。读取、目录和文件正文使用独立响应，业务错误沿 Core Problem 合同；开始发送文件后发生错误则终止流。Core Idempotency-Key 只去重本地命令；操作引用和本地修订不提供远端创建归属或迟到写入保证。

### 11.2 不改写既有 Snapshot

保留 Client API v1 现有端点语义、Snapshot v4/v5 及历史不可变 Release 字节/hash。新增可选能力通过独立接口获取，不向旧严格 DTO 中直接插入字段，也不为单个用户开通重写共享 Snapshot。Admin 发布绑定、内部控制和预算/用量的新增分支是明确的协议变更，须更新 baseline、版本/能力门禁及支持矩阵，不能用“客户端 Snapshot 不变”声称所有 API 无变更。Portal Bridge 此次无变更；文件能力不通过 Portal Cookie 或 Bridge 偷渡授权。

新 Android 在已确认的 Core 来源查询新增工作区端点；只有成功校验该接口的版本化投影后才启用新功能。新 Core 无工作区应返回结构化未开通状态，不能与未知路径混淆。旧 Core 没有新增能力的权威否定响应，单独一个 404 无法可靠区分旧路由和代理故障；此时保持“未确认支持”、不注册文件功能，保留诊断并允许后续重试，既有企业功能继续运行。401/403 沿身份错误处理，超时/格式错误表示不可用，不写成永久未支持。若增加 Discovery 字段，必须另做旧消费者验证，首期不依赖该修改。

| 组合 | 支持边界 |
|---|---|
| 新 Core + 默认未配置/从未启用集成 | 旧功能维持原行为，无远端账号副作用；已启用集成的停用按 5.3 执行 |
| 新 Android + 旧 Core | 无远程工作区界面，既有企业能力继续工作；新 APK仍支持旧 Snapshot |
| 旧 Android + 新 Core | 既有 Snapshot 可解析；无新文件界面；通过现有 MCP 调用工作区须有真实旧版本协议测试才能列入兼容名单 |
| 新 Core + 旧 Relay | 普通旧配置可按既有支持范围运行；禁止启用按用户绑定，不能忽略扩展降级转发 |
| 新 Core + 具备管理 API/MCP/DAV 及指定 token 合同的 Agent Space | 按该版本接入，无需进一步改造远端；DAV 关闭时文件不可用，MCP 独立判断 |
| 新 Core + 缺少管理 API/WebDAV 或指定 token 的历史部署 | 明确缺少能力，按独立服务发布手册准备对应版本；文件功能不静默降级为远端随机签发 |
| 新 Core + 已验证的现有 Agent Space + 新 Android | 完整工作区识别、MCP 和文件闭环；外部 DAV 客户端另做接入验收 |

一般网络错误与权限错误保留稳定 reason/code，不以自然语言解析。分别表达未开通、已断开、集成不可用、空间不匹配、凭据不可用、删除中、条件冲突、容量不足、结果未知；最终代码集中在对应合同。既有 user_disabled/device_revoked/session_revoked/enterprise_identity_deleted 与 generation barrier 不改变含义，远端凭据失败与平台身份失败分开。

错误处理必须覆盖：本地绑定版本冲突重新查询意图；文件 ETag 冲突让用户确认覆盖；空间失配停止自动操作；管理认证失败保留配置但拒绝继续执行；远端未知状态不重建；目录部分失败保留已完成项；响应丢失保留操作待核实。Core 将远端诊断关联到本地操作/请求并脱敏保存，不要求 Agent Space 提供新的关联字段；客户端只见稳定分类。

## 12. 持久化、升级与恢复

### 12.1 Core 数据

新增持久结构仅覆盖真实新事实，命名由实现仓库确定：

- WorkspaceService 与不可变配置修订：类型、候选/生效配置、管理/MCP/DAV 目标、Secret 版本引用。
- 用户工作区绑定：userId、workspaceServiceId、remoteUsername、spc、连接意图、MCP/DAV 有效凭据引用及可用状态、本地绑定修订、远端观测及已应用控制修订。
- 未完成操作：本地命令引用、原配置/秘密版本及目标、本地意图版本、DAV 候选 Secret 引用、步骤发送/确认状态、最近错误及核实信息；复用现有持久化/Idempotency 和后台处理，远端操作不能冒充单次 Relay Activation，不另建 DAV 凭据状态表。
- 预算/用量的目标归属：增加集成分支所需字段和约束，历史 upstreamId 不改写；不以重新计算旧事件/hash 迁移数据。

设置用户未删除空间唯一约束、集成/空间归属唯一约束和并发版本检查。企业用户删除所需清理记录必须与用户行级联生命周期分离，完成后删除其私有数据，仅保留合同允许的审计/不可逆身份墓碑。

严格遵守追加式 SQL migration：不编辑旧文件，Ent 与 SQL 同步；升级默认无启用集成，不批量创建空间。用户、设备、Session、Secret、Draft、历史 Release/hash、Usage 和预算保持。数据库配置引用历史 secretVersion，不能读取“最新密钥”悄悄改变已应用状态。

### 12.2 现有远端部署与历史账号

本次不修改 Agent Space 状态 schema，不安排服务身份、创建关联或账号修订迁移。接入前确认管理 API、MCP、WebDAV 和指定 token 合同的支持版本。当前部署手册的已发布起点是静态 `[users]`、四个 MCP 工具、旧 `/mcp` 入口；仅支持该基准导入及完整卷备份/独立卷回滚，不再维护未发布动态状态中间版本的迁移入口。静态导入保留用户名和实际磁盘资源，新分配 spc，不导入旧 Token；部署方须重新签发凭据并更新客户端入口，Core 接管时保存导入后的 spc。不将历史 schema1 测试当作当前接入要求。确切源镜像身份及步骤以 `docs/docker-deployment.md` 为准；升级由 Agent Space 部署方执行，Core 不执行 init-state 或改写远端状态。

新 `measix_<uuid>` 只用于新开通账号。现有裸 usr 或人工账号按 4.2 显式接管，保留 username、spc 和磁盘；不批量改名、重建或自动认领。本映射是 Core 集成约定，不要求修改 Agent Space 通用账号规则或其仓库文档。

### 12.3 部署次序与回滚

推荐顺序：备份并验证恢复能力 → 准备 Agent Space 已完成独立验证、支持指定 DAV token 的版本及管理/MCP/DAV 配置 → 升级兼容的新 Relay → Core migrate/check → 启动新版 Hub/Admin → 验证原功能 → 保存并启用远程工作区服务 → 显式开通测试用户 → 验证 Core 文件及管理员交付后的 DAV 直连 → 按需添加并发布 MCP、验证工具 → Android 适配/兼容验证 → 扩大开通范围。旧部署可能需要升级到该支持版本，本方案不再要求 Agent Space 增加其他接口或 schema。

Core 继续显式执行 `control-hub migrate`、`control-hub check`，Hub 启动不自动修改 schema。部署包声明最低组件协议能力，具体命令和受支持起点由发行手册固定；不能依赖某个会变化的分支或相同 SDK 标签。

功能回退优先停用集成并保留数据。二进制回退必须确认旧程序能否读取新 schema；否则停服恢复经验证的一致备份。Core 备份包含数据库和必要密钥，Relay spool 按自身所有者处理；Agent Space 备份包含状态与双盘。恢复后核对撤销、轮换、删除和新建事实，不能因恢复旧状态复活旧 Token 或把 Core 绑定指向另一空间。

两项目不要求锁步发布，只按版本化接口协作。升级/恢复测试使用真实代表性历史数据，不以空库通过代替，也不把本地候选验收当成生产验收。

## 13. 文档与代码落点

| 所在仓库 | 本次应更新内容 |
|---|---|
| measix-architecture | 术语中的远程工作区产品名与新集成标识；S1 接入合同；Hub/Relay 的按用户绑定和文件职责；Admin/Android 工作流；版本/迁移/撤销语义及测试要求；阶段索引 |
| Core | Admin/Client/Relay/Usage OpenAPI、fixtures、generated artifacts、协议 baseline；Ent/追加迁移；集成与工作区服务、Agent Space adapter、runtimecontrol/Relay、预算/Usage 归属、文件 API；Admin UI、预览与操作诊断 |
| Agent Space | 无仓库改动；只作为现有外部服务核对接口、部署配置与实际集成结果 |
| Android | 导出合同的实际 Kotlin 消费、Enterprise 工作区投影、既有 MCP 接入、文件/预览 UI、退出取消、必要持久迁移和真机验收 |

Core 的具体包拆分保持直接：集成配置、工作区操作及一个 Agent Space adapter（管理 API 与标准 DAV）；文件 UI 经应用服务访问。不预先创建 provider 插件框架、通用任务平台或另一套配置总线。

先在架构层冻结本文涉及的新增语义，再落 executable 合同。方案中的路径/字段草案不能被当作绕过该顺序的接口权威。已实现后把事实归入对应当前参考文档，本文保留实施决策和交付记录，避免长期与运行文档重复维护。

## 14. 实施阶段与验收

| 阶段 | 交付内容 | 阶段退出条件 |
|---|---|---|
| P0 合同 | 术语、项目边界、热更新、生命周期、目标绑定及预算/用量、文件/预览、兼容和迁移 | 架构与本方案无冲突；冻结接口/共享案例及组件门禁；不遗留“断开即删除”等歧义 |
| P1 Core 基础 | 新表/迁移、集成 Admin、Secret、check/apply/reconcile | 无重启应用、冲突/未知结果如实显示、原功能不回归 |
| P2 用户与 MCP | 用户操作、发布关联、每用户 Relay 绑定及撤销、预算/用量归属 | 两用户真实 MCP 隔离、原 spc 保留、旧会话失效、本地幂等及可确认步骤恢复、准入与用量闭环 |
| P3 Admin 文件 | DAV adapter、文件/预览 API、审计及 Admin 连接信息 | 先存后设、指定值核对、同值恢复边界；真实浏览器上传/落盘/预览、权限和 DAV 客户端直连；查看不轮换、撤销不自动签发 |
| P4 Android | 状态识别先交付，随后文件与预览界面 | 新旧组合、真实设备传输、切域/退出、附件闭环和持久迁移 |
| P5 升级交付 | 历史部署演练、备份恢复、发布包和证据 | 固定源码/镜像/合同/客户端版本的完整证据及明确限制 |

P0 是合同落地工作，不是实施中任意选择语义的占位阶段。退出前必须确定：Core 新增接口的版本/方法/路径、本地幂等与错误 schema；目标绑定与预算/用量分支的版本；文件元信息/部分结果/条件头与传输限制配置；客户端投影/旧服务探测；用户删除清理记录；各历史迁移起点。普通库选型、包名和具体 UI 排版由各实现仓库决定。P1 完成配置保存、启用和用户生命周期；文件按 P3 独立验收，P2 的 MCP 发布与每用户转发是可选后续工具链路。不设置 Agent Space 改造阶段。

以下矩阵是本实施方案检查清单，不冒充已登记的架构测试 ID：

| 范围 | 必测场景 |
|---|---|
| 独立性 | Agent Space 无 Core 可启动和使用；Core 未配置集成时原功能可用；Core 不接触远端状态/磁盘 |
| 热更新 | 配置 CAS、保存未应用、无 Release 时首次配置、目标核对失败、认证失败、Relay 已应用但确认丢失、Hub/Relay 重启、普通切换与安全撤销不同处理 |
| 账号/归属 | 固定前缀、两个独立 Core 部署用户不冲突、同名既有账号拒绝自动接管、历史账号接管、数据库克隆冲突明确处理 |
| 开通恢复 | 创建冲突/响应丢失不自动认领、未知创建取消不宣告完成；发送标记前/后、响应后事务提交前/后崩溃；MCP 响应凭据原子保存、DAV 候选先存后设、迟到结果不覆盖新意图；未知写入阻止冲突步骤和目标迁移、已确认任务继续轮询、人工处理保留依据 |
| 发布与计量 | MCP 撤下只撤销 MCP、重新发布不复活已断开绑定、首次发布不夹带无关草稿、预算拒绝与 Usage 归属、旧 spool 重放、新旧目标历史读取 |
| 用户隔离 | A 的 MCP session/path/文件请求不可访问 B；伪造 userId/Header 无效；无绑定绝不使用共享凭据 |
| 删除 | 删除 202 与完成区分、部分资源清理失败、原 spc 失配、同名重建、删除用户与保存清理目标事务失败/恢复、用户已删仍能收尾且展示远端进度 |
| DAV 凭据与直连 | 候选保存失败不发送、重启沿同一候选继续、响应丢失不换随机值、确认前不展示候选；当前同值设置不取消传输，过期操作不重放，旧请求未收敛不推进后续改值或宣告撤销完成；400/409 不降级、返回 token 不匹配拒绝生效；Admin 查看/复制与撤销复验、查看不轮换、Basic/Bearer 完整值及外部客户端真实使用；恢复/重新启用不自动签发，显式新值设置后重新交付；Core 登出不撤销直连、直连不冒充 Core 审计 |
| 文件准入 | disabled/stopPending/deleting 时 Core 用户和 Admin 均拒绝文件请求；DAV 关闭时文件不可用且不旁路；MCP 可用性独立判断 |
| 文件安全 | 根与别名保护、编码穿越、symlink/特殊文件/保留区拒绝、跨空间目标；源/目标 ETag 分别验证，目标变化拒绝覆盖，目录目标已存在不预删；207/propstat 部分失败与恶意 XML/href；浏览器身份/来源头不泄漏，禁止重定向，远端 401 不触发 Core 登出 |
| 文件可靠性 | 至少 64 MiB 往返摘要、慢传输、Range/ETag、磁盘不足、提交前断流保留旧文件/提交后响应丢失结果未知、取消等待/在途 IO、Hub 内存有界；保留 shared arena 失败诊断 |
| 预览 | 四类格式真实打开；Markdown 原文/相对图片与恶意链接；图片解码限制；PDF 多页/Range/版本变化；HTML/SVG 下载不执行 |
| Core 访问撤销 | 撤销与登记竞态、旧文件请求在断开再恢复后迟到、预览/PDF 分段和传输期间撤销；Admin 失效/客户端切企业与登出；原 Session 租约取消、旧视图/附件迟到结果；瞬时失败不清除 MCP 工具目录；授权先于 304；不要求远端新增账号修订检查 |
| 协议兼容 | v4/v5 历史 Snapshot 字节/hash 不变；新端点在旧 Core 缺失；旧 Android 实际 MCP；旧 Relay 禁止忽略按用户绑定；Agent Space 指定 token 响应必须逐字匹配，配置不包含第三方提交号或人工能力声明 |
| 数据迁移 | Core 空库与支持历史版本升级、重复迁移、非法历史拒绝、绑定/凭据/发布保存；按独立服务已有流程验证备份恢复，不增加远端 schema 迁移 |

代码行为变更按各仓库要求执行 Red → Green → Refactor；文档变更不造人工业务测试。Core 使用真实 SQLite/HTTP/Relay 与确定性夹具验证异常和恢复，再执行生产构建 Admin 的浏览器闭环及用户 DAV 客户端直连。远端使用 Agent Space 自身已发布验证、包含指定 token 能力的固定镜像/helper，实际文件访问验证 VM 链路，不另行改造该项目。Android 生成类型不等于实际客户端已适配。

每阶段记录 exact commit、未提交输入摘要（如有）、OpenAPI/fixture hash、Agent Space 镜像与 helper 身份、APK/浏览器版本、执行命令、结果及未测项。当前 Agent Space 207 项测试和 Docker/rclone 记录不能替代 Core 新增集成的候选验证；不得据此宣称生产或完整 S1 通过。

## 15. 参考与待同步文档

本轮客户端文件能力补齐采用现有 owner 和接口：独立投影增加企业服务状态，所有文件请求固定空间 ID，补齐流式传输、条件写、Range 和错误合同；Admin 增加 UTF-8 文本新建/编辑/另存及预览下载。分享采用客户端下载副本后交给系统分享，不新增服务端公开分享、编辑会话或锁服务。该工作区协议尚未生产发布，直接同步 Core/Admin/导出合同，不引入旧工作区版本回退。当前实现与验证结果分别见 [实现参考](remote-workspace-implementation.md) 和 [联调记录](remote-workspace-verification.md)，Android 原生功能仍由客户端单独实施。

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

下列路径相对 `/home/hualei/msb_demo`，只读参考其当前接口与部署边界，本次不修改该项目：

- `docs/agent-space-architecture.md`：当前状态、运行与入口权威。
- `docs/hosted-workspace-reference.md`：现有管理/MCP/DAV 合同；其中旧 MEASIX 账号映射说明不作为本次 Core 映射规则。
- `docs/workspace-webdav-plan.md`：文件操作、原子性、取消、迁移合同。
- `docs/managed-guest-services.md`：helper 分发、实例归属和升级边界。
- `docs/webdav-execution.md`：具体候选验证身份及未验证客户端范围。
- `docs/docker-deployment.md`：当前静态用户升级、备份回滚与运行流程；已删除的旧 schema2 升级专页不再作为入口。

Android 实施前重读其 `AGENTS.md` 和 `docs/references/mcp-architecture.md`、企业配置/持久化/附件相关当前参考文档。本文不取代 Android 的 owner 和迁移合同。
