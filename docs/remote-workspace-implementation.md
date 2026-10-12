# 远程工作区集成实现

本文是 Core/Relay/Admin 当前源码的实现参考。跨组件语义与验收边界以 [S1 合同](../../measix-architecture/docs/10-runtime-foundation/s1/measix-s1-remote-workspace-contract-spec.md) 为准。Android 原生实现由其仓库维护；Core 导出不替代设备证据，也不证明完整 S1 compute/storage 计量或生产部署。

## 所有者和入口

| 所有者 | 源码 / 责任 |
|---|---|
| Hub workspace | `backend/internal/hub/workspace/`：服务配置修订、用户连接意图、持久操作、文件租约与审计 |
| Agent Space adapter | `backend/internal/hub/agentspace/`：固定管理 API、MCP/DAV origin 校验、条件文件操作、流式 IO；不读取远端状态目录或磁盘 |
| Runtime Control | `backend/internal/hub/runtimecontrol/workspace.go`：复用原全局 Activation、revision/hash/ACK；工作区不另建运行状态写入者 |
| Relay | `internal/relay/control/`、`runtime/`：按已验证用户及 MCP / DAV 资源选择固定空间绑定，取消失效在途请求；DAV 标准转发和基础统计独立于模型准入 |
| Secret | `internal/hub/upstream/secret_transaction.go`：在业务事务中复用 SecretBox/SecretVersion；不建立派生密钥或第二份凭据表 |
| Admin | `RemoteWorkspacesPage.vue`、`WorkspacePanel.vue`、`WorkspaceFiles.vue`、`WorkspacePreview.vue`、`WorkspaceTextEditor.vue`：全部通过同源 Admin API |
| 第三方服务 | Agent Space 拥有账号、空间、VM、磁盘和文件；独立发布并以管理/MCP/DAV 合同接入 |

当前适配 Agent Space 的管理 v1、资源摘要、MCP 和 WebDAV 合同。Admin 填写地址和管理凭据后即可保存并启用，不要求人工声明版本或能力；配置不包含第三方提交号，也不按固定提交号设置运行门禁。连接检查和实际请求校验响应、空间身份与凭据，失败按对应合同处理。历史测试源码和镜像身份由[证据索引](s0-execution-progress.md#历史证据入口)定位，当前联调须重新固定输入。管理凭据保存为版本化 Secret 引用。

新绑定把 `usr_<uuid>` 映射为远端 `measix_<uuid>`，Hub 持久保存实际 remoteUsername 与 agentSpaceId。用户显示名/本地 username 修改不重命名远端；历史账号只能经明确核实/接管关联原空间，不从同名账号推断身份。普通配置迁移继续核对已绑定原目标，不能重建空间替代恢复；旧部署已清理后的显式更换见下方修复流程，新空间不代表旧文件恢复。

## DAV 未知操作的断开恢复

DAV 设置或撤销停在 `DAV_SENT` / `DAV_REVOKE_SENT` 且结果未知时，核实旧请求已结束后可使用“断开”退出恢复阻断。此时必须填写核实依据；后端在同一事务校验原操作、绑定版本、配置与账号/空间身份，结束旧操作并建立正常断开流程，不重放 DAV 写入。文件和空间保留，访问撤销确认后才可修改错误配置并恢复连接。创建/启用结果未知及缺少依据的请求仍保持阻断。原操作固定地址不随配置修改，普通“核实后继续”也不会修正错误地址。

## 管理员资源摘要

`GET /api/admin/v1/users/{userId}/workspace/resources?agentSpaceId=spc_…` 独立于原工作区投影，只有 Admin API 提供，响应禁止缓存。Hub 使用当前已生效配置的管理员凭据查询 Agent Space 用户详情；不会调用 DAV/MCP，不要求工具发布或文件凭据。服务关闭、用户停用、工作区断开后，已知绑定仍可核查。未开通时页面隐藏摘要，后端不猜测远端账号或采集目标。

请求前后复验管理员身份、原空间、绑定修订/状态/意图和配置修订/启用状态。409 表示目标或配置变化，页面清除旧结果；远端错误不修改连接状态、文件凭据或持久观测。Core 只投影固定字段，各项独立解析和校验；缺失、数值越界或 JSON 类型错误只使对应项失效，保留其他正常观测和有效零值。远端诊断只允许合同原因码，不透传任意文字。查询总超时 15 秒，原生命周期操作和列表不增加新的资源查询。Agent Space 当前普通用户详情 GET 自带采集，已有管理核查会承担其有界耗时；本轮未为此修改独立项目。

`WorkspaceResources.vue` 由两个入口的 `WorkspacePanel` 共用：运行状态、工作卷已用/可用、客户机内存三张卡片，另列 CPU 核数和内存/工作卷配置及来源。当前/历史/暂无数据/采集失败逐项呈现，时间采用原 Unix 毫秒；客户机内存不表示宿主 RSS，配置容量不当作文件系统可用容量。未创建 VM 时明确标注创建默认值，不提供 CPU 使用率或利用率图表。

打开面板加载摘要，可手动刷新；页面可见时每 15 秒串行请求，隐藏、切用户、空间或生命周期上下文变化时取消原请求并丢弃迟到响应。传输失败时保留的数值标为历史；身份/权限冲突清除旧摘要。资源错误独立显示，不覆盖文件编辑内容。容量格式与文件区共用，非零小容量不四舍五入为 0 MiB。没有资源数据库、后台采集器或第二套权限所有者；资源摘要本身不扩展 Snapshot v5、Client 投影或 Relay 协议。

## 配置、发布和用户生命周期

1. 在“远程工作区”（`/admin/remote-workspaces`）打开启用开关，填写管理服务地址、管理凭据和可选的独立 DAV 地址，点击“保存并连接”。开关仅表达待保存意图，摘要显示实际状态及“新配置未生效”。MCP 原地址与管理地址相同时默认随其更新；独立地址保留在可选配置中。“检查连接”查询已保存版本，不因生效版本滞后而禁用。
2. 页面保存不可变配置修订，检查管理连接和所有原空间，再提交带 expectedConfigRevision 的启用操作。应用 API 必须携带该修订，缺失或无效返回 400，修订变化返回 409；不以空正文应用当前版本。地址保存只准备配置；应用仍核对原账号及原空间 ID，不能重绑新身份。服务 API 位于 `/api/admin/v1/remote-workspace/services`；配置保存本身不发布能力。首次成功启用已配置 DAV 的服务后，自动将默认开启的 DAV 及固定来源绑定加入现有草稿，保留其他编辑；管理员仍按原流程发布。以后应用配置不覆盖显式关闭的 DAV。
3. 服务启用后选择有效用户开通空间，也可在用户详情的“远程工作区”操作。开通和文件管理无需发布 MCP。首次配置 DAV 后，确认有效凭据才显示“浏览文件”；打开详情本身不启动 VM。
4. 需要助手工具时，进入“企业配置 → MCP”，显式“添加远程工作区 MCP”，再按原流程审查、发布。草稿存在未保存修改时先保存或放弃，避免覆盖。共享 Snapshot 包含稳定资源定义，不包含用户空间或上游凭据。已有空间在发布后获得工具入口，无需重新开通。
5. 关闭开关后仍需点击“保存并关闭”并确认。关闭会撤销访问并停止工作区，保留文件。未开通用户隐藏工作区页签，已有空间保留核查和删除入口；MCP 页面隐藏新增入口，但保留现有定义和服务已关闭提示。
6. 断开先撤销 Core/Relay 访问，再禁用原远端账号并等待 stopPending 收敛；文件保留。恢复启用原空间并重新签发 MCP；持久文件授权为 AUTHORIZED 时自动准备 DAV，REVOKED 保持撤销。正常重启复用已确认凭据，无人为有效期和定时轮换。MCP 凭据维护、客户端 DAV 权限和原始外部连接信息分别呈现。
7. 停用用户或服务保留原连接意图；重新启用仅恢复此前 CONNECTED 意图，手动断开保持断开。删除用户在原用户清理事务中保留无用户外键的远端清理目标。远端删除 202 后仍需查询确认完成。

打开工作区详情不读取文件、不启动 VM；点击“浏览文件”或实际执行工具才可能启动运行环境。MCP 和文件可用性分别投影。DAV 返回认证失败仅使该版本 DAV 不可用，不触发 Core 登出或清除 MCP 能力。

## 操作与恢复

`WorkspaceOperation` 保存 actor、幂等键、固定目标、配置/绑定修订、步骤、诊断和核实依据。管理写入前提交 `*_SENT`；重启见到 SENT 进入 UNKNOWN，不自动重放。原 Hub reconcile 循环投递到一个有界工作执行器，远端 IO 不占住全局 Activation 事务。

### 独立服务已重建后的修复

只有管理状态通过认证、所有原账号均返回结构化 `not_found`，且无未结束工作区操作或删除意图时，`check` 才提供更换预览。部分缺失、普通路由 404、身份不同、认证失败及未知请求均不提供此入口。普通“保存并连接”不会重新开通；用户必须在“修复连接”确认旧部署与旧数据已清理、旧管理请求已结束及新空间为空。

`POST .../services/{id}/replace` 用配置修订、完整用户/空间清单、MCP 绑定及独立 DAV 修订做 CAS。原绑定完整元数据归档到同一持久 `REPLACE` 操作目标；先撤除 Core 准入，经原 Activation ACK 后，在一个事务里更换当前绑定并排入普通 CREATE 子操作。只自动开通原 CONNECTED 意图的有效用户；保持原 DAV 权限和文件授权意图，REVOKED 不恢复。新绑定修订单调递增，服务、MCP/DAV 资源身份保留，用户、草稿、发布原字节及用量不改写。归档不是文件备份，Core 不拥有远端卷。

父操作的 RECREATING 进度来自原子记录的子操作 ID；重启沿同一队列继续，不另设循环或表。未知创建不重发，管理员在对应用户面板核实并显式接管后，父操作跟随该接管的确认结果；其他原有查询/继续规则仍适用。没有对旧远端发 DELETE/STOP，也不能据本地撤除声称旧外部凭据已在旧部署失效。网页网络/5xx 未确认时保留原路径、正文和幂等键，核实原操作不会变成另一轮开通。

排队事务遇到暂时的 Activation 争用时回滚并保留 REMOTE 步骤，复用原 reconcile 重试。并发用户删除的旧绑定保留给原身份清理；已完成清理且用户不存在时跳过，不重建该用户，也不阻塞其他子操作。只读预检查期间若同一幂等请求已提交，先返回原操作再解释已过时的检查结果。冻结的网页操作重试直接使用原请求，不依赖当前服务缓存仍可读取。

- **创建响应丢失**：重新查询并核实用户名及原空间；确认旧管理请求结束后显式接管。不得重新创建或把同名账号直接标为已连接。
- **MCP 写入结果未知**：核实旧请求结束后继续，恢复可编译状态再走原 Activation。明文只能由成功管理响应写入 SecretVersion。
- **DAV 设置**：随机候选值先在同一事务持久化，再发送指定 token；响应必须逐字匹配。候选未确认不交付。显式继续使用原候选，不生成新随机值。DAV 变更不修改 MCP binding revision。
- **用户停用/删除**：已知空间的凭据操作可由当前撤销意图替代，保留原 UNKNOWN 证据；远端禁用状态阻止迟到的凭据设置重新授权。尚可能在途的 CREATE/ENABLE 不能按此方式跳过，显示 `revocation_requires_remote_confirmation`，管理员确认原请求结束后继续当前清理，不重放启用。
- **未知创建后的清理**：已停用/删除用户可在“核实后继续”填写原账号、原空间及核实依据。Core 验证与原创建目标一致后仅继续停用/删除；仍有资格的用户走显式接管，不直接恢复未知凭据。
- **管理凭据失效**：上述继续操作可填写新的管理凭据，先保存为 Secret 引用，再核对原目标。仅替换当前操作引用，不改服务地址或有效服务配置；操作收敛后再同步服务连接配置。
- **启用和控制失败**：只读启用检查失败恢复操作前的启用状态，已关闭服务不会被误启用。Relay 控制失败保留原工作区操作及绑定意图，由原 Activation 持续对账；不重发已确认的远端创建或凭据写入。
- **重启恢复**：恢复固定 descriptor 前复验当前 workspace 意图、目标、配置及 SecretVersion；已撤销意图不得复活。新工作区命令与全局待确认 Activation 互斥。控制 v3 要求 Relay status 和 ACK 明确支持其 DAV 绑定版本；保留无 DAV 的既有协议恢复路径。
- **控制意图提交**：编译在持久事务之前完成；保存 Activation 的事务重新核对完整工作区 MCP/DAV 绑定集合及连接、响应头、空闲超时。编译后权限、凭据、连接或配置发生变化时返回冲突，不持久化过时意图；该检查不放宽历史 descriptor 重放的版本与授权校验。
- **远端已删除、同名重建或身份不匹配**：保留诊断及原目标，不能改绑新 `spc_*`。人工核实不是后台自动恢复的替代名词。

管理 API 不提供远端请求幂等键或条件账号修订，因此未知创建/启用仍有明确的人工收敛边界。不要通过修改数据库跳过操作，或在旧请求未确认结束前宣告撤销完成。

## 文件、预览与审计

远程工作区页面将服务连接收为可折叠区域，摘要保留实际状态；未配置时直接展开。用户选择和工作区详情使用页面全宽，文件入口前置，文件区位于权限和容量明细之前。连接维护、DAV 权限及资源摘要分别展开，避免日常浏览被配置表单和长标识挤占。详情内部不再并排堆放文件与管理卡片；用户详情入口沿用同一面板。

空间标题显示“已开通”，不把准入投影显示为绿色实时健康。文件服务暂不可用的诊断留在文件区域，允许手动刷新读取，未知写入仍要求核实。Agent Space 的空正文 503 不提供具体容量原因；Core 不猜测活动上限，也不在此界面修改独立服务容量或重启其容器。

`WorkspaceFiles.vue` 的“筛选当前目录”只按已加载条目的文件名在本地过滤，并保持目录优先排序，不递归查询远端、不代表全空间搜索。切目录清空筛选，显示筛选后的条目数量；操作仍携带所选条目的原路径和版本条件。

Admin `/api/admin/v1/users/{userId}/workspace` 和 Client `/api/client/v1/workspace` 下的 `files`、`content` 进入同一个应用服务。Client 用户来自已验证令牌，不接受请求提供的 userId；Admin 每次验证会话、角色及写请求 CSRF。

全部文件请求必须带 `agentSpaceId` query，取自当前 WorkspaceProjection。它固定本次操作目标，不提供选择其他用户的权限；参数缺失/无效返回 400，当前空间与目标不一致返回 409 `workspace_space_mismatch`，即使当前文件服务不可用也先区分空间不匹配。不能把旧页面、草稿或排队写入提交到重新开通的新空间。

- `GET files?path=` 列目录、元信息和卷容量；`POST files` 仅接受 MKCOL/MOVE/COPY/DELETE 的类型化 JSON。
- `GET/HEAD content?path=` 返回流、ETag、Range 元信息；`PUT content` 直接流式传输，创建要求 `If-None-Match: *`，覆盖要求单个源 `If-Match`。
- COPY/MOVE 分别校验源 ETag、目标 tagged If；普通文件覆盖需目标 ETag，目录覆盖禁止。目录删除需显式递归确认。不把 DAV Destination/If 或浏览器 Cookie/Origin 任意转发。
- 相对路径禁止根修改、编码穿越和反斜线。`FilePath` 仅为 Files 增加根下精确名称 `.agent-space` 及其子路径的保护，覆盖读取、上传、修改源和复制/移动目标；相似名称及普通子目录中的同名路径正常处理。列举先验证上游 href 的授权根、通用路径和浅层关系，再跳过保护项；根容量信息保留，非法上游路径使用 `invalid_dav_href` 而非用户输入错误。DAV XML 有大小/数量限制，拒绝指令和跨账号/跨 origin href；写操作 207 的失败项保留为有界 PARTIAL/UNKNOWN 结果。
- PROPFIND 裸 opaque ETag 规范化为 HTTP 引号形式，随后仍由远端条件请求验证。404 必须通过已认证根目录探测区分文件不存在和文件服务失效。
- 并发文件请求最多 8；连接/响应头与传输无进展超时受服务配置控制，目录及多状态 XML 响应正文同样受空闲超时约束。请求取消、会话撤销、用户/服务停用、绑定或凭据修订变化均取消原租约。旧请求失败不能撤销新 Token。
- 审计保存 actor、用户、原空间、路径、实际操作、字节数和结束结果；外部 DAV 直连不冒充 Core 审计。连接信息领取是显式 POST/no-store；列表不返回明文，查看不轮换。
- 未知文件写入产生 `workspace.file_write_unknown` 结构化诊断，仅记录动作、远端错误分类/状态、字节数及请求/租约是否取消；不记录路径、URL、正文、原始错误或凭据。结合审计区分取消和远端失败，不据日志自动重放。
- 网页下载通过原生浏览器下载，不把整个大文件读入 JavaScript。上传显示进度并可取消；断流提示刷新核实，不自动重试写入。
- 文件服务地址未配置时不提供 Token 签发入口，后端在持久化操作前拒绝该请求。上传更换文件、目录或刷新发现目标版本变化时，必须重新确认覆盖。

预览纯文本/Markdown 上限 2 MiB，PDF/图片上限 24 MiB；图片头在解码前限制 1600 万像素，PDF 每次渲染画布同样有像素上限。Markdown 禁用原始 HTML 执行、外部图片和危险链接，原文可切换，相对图片通过当前授权文件 API 加载（每张 4 MiB，最多 20 张）。PDF 使用 pdf.js 模块 worker 和 canvas，支持翻页/缩放；不执行文件作为网页。HTML/SVG 等只下载。关闭预览会取消读取、渲染并释放对象 URL。

视频使用原生 `<video>`，不读入完整 Blob、不转码。MP4/M4V/WebM/OGV 通过同源 Files GET/HEAD 的 `preview=video&expectedEtag=<strong ETag>` 获取固定媒体类型、inline 和 nosniff；Hub 将版本转换为 If-Match，Range/If-Range 仍由上游判断。缺少强 ETag 不启动预览，版本变化返回 412；浏览器不支持的编解码可下载。关闭、切用户/空间或撤销访问时取消媒体读取；普通下载及 HTML/SVG 仍为 attachment。

## 标准 WebDAV 资源

服务持久拥有一个 `dav_*` 资源和 `rte_*` 路由。企业配置 WebDAV 分区只编辑名称和开关，不重复添加、删除或改来源；保存、校验、差异审查、快照预览及发布沿用原流程。旧编辑器漏传派生定义时拒绝保存，避免静默删除。HTTP/HTTPS、本机、局域网及 IPv4/IPv6 均受支持，不增加私网白名单。

Snapshot v5 的可选 `webdav[]` 只包含稳定资源定义、`WORKSPACE_FILE_EXCHANGE` 用途及平台 endpoint 模板；来源服务由 Hub 的派生资源和私有 RuntimeBinding 关联，不在共享 Snapshot 中下发上游配置。当前用户的 `WorkspaceProjection.webdav` 返回实际可用性和生效 READ_ONLY/FULL；Admin 另有 desired/lastConfirmed 修订，Client 移除这些管理字段。唯一 `fileExchangeWebdavResourceId` 仅在服务启用且 DAV 已发布启用时指定；临时不可用保留指定身份但不得执行，服务或资源关闭则取消指定，客户端不另选 DAV。权限或控制结果尚未确认时不报告生效权限。

平台路径为 `/runtime/v1/resources/{davId}/spaces/{agentSpaceId}/dav/`。客户端使用可注入平台 Bearer 的标准 DAV 库；仅新请求刷新短期 access token，不给第三方服务器发送平台 token。请求开始验证 JWT，活动传输的绝对上界为签名 sessionValidUntil；短 JWT 到期不截断已经准入的大文件，用户/会话/密钥/资源/绑定撤销仍即时取消。无 generation/interaction 私有头、模型 body 上限、预算或总传输时长限制。

Relay 注册全部 RFC 4918 方法，双向流式转发、有界并发和缓冲、背压及双向空闲取消。READ_ONLY 只允许 OPTIONS/GET/HEAD/PROPFIND；FULL 允许约定的标准 DAV 方法，上游是否支持仍由其状态回答。仅映射受控根内 Destination、tagged If、Location/Content-Location 和协议资源 href；锁 token、dead property 与普通文件正文保持不透明。上游 401 转换为明确 502 工作区凭据不可用，不触发平台登出；普通 403、207、条件状态、Range、锁及覆盖语义透传。

标准 DAV 不应用 Files 的 `.agent-space` 名称保护：目录项完整保留，根内路径及 Destination 正常映射，访问和修改由 READ_ONLY/FULL 及上游决定。根修改、跨空间、穿越及非法编码仍拒绝；无需在 XML 中缓存或过滤目录项。

协议 XML 支持 UTF-8/UTF-16、gzip，逐 token 保留原命名空间/扩展正文，修改后清除旧长度和摘要。单 token 上限 1 MiB、层级 256、资源 href 16 KiB，读取阶段也限制缓冲；整份目录不套 Files 的 4 MiB/2000 项上限。中途非法 XML 终止流，不追加 JSON；普通文件下载不经过 XML 解码。

DAV 有独立绑定修订和 SecretRef。签发/重设/撤销的撤除与恢复分别使用持久 Activation 阶段，匹配 revision/hash/ACK 后才完成；权限变更不轮换上游 key、不改变 MCP 修订，也不改变 Console Files 权限。当前版本上游 401 经原统计管道持久回报并撤除绑定，不能凭 401 自动覆盖可能的外部改钥；旧事件不影响新凭据，管理员核实后重设。首次缺少凭据及已确认禁用后的恢复自动处理，未知远端写不重放。

仅 DAV 记录 method、状态、开始/结束、时长及请求/响应实际传输字节。事实标记 `statisticsOnly`，复用 spool、投递、删除屏障和去重；没有预算准入、语义 meter、费用或 DAV 配额。Console DAV 详情/筛选隐藏费用与额度。Files 保留原操作审计，不加入统一用量；管理员领取原始直连 key 不受平台 DAV 权限/开关/统计约束，界面明确提示。

文本编辑复用 GET/PUT，不新增服务端编辑会话。正文与强 ETag 必须来自同一次 GET；新建/另存使用 `If-None-Match: *`，原文件保存使用该 GET 的 `If-Match`。Admin 编辑严格 UTF-8、最多 2 MiB，保留 BOM 和统一换行风格；二进制、非 UTF-8 或混合换行不自动转码覆盖。成功后关闭编辑器，下一次编辑重新 GET，避免把保存之后其他写入的版本错误地绑定到旧缓冲区。冲突/未知结果保留文字、禁止直接重放原写入；同名另存失败也需先核实或换新目标。关闭、重新读取和路由离开对未保存文字要求确认，页面刷新/关闭有浏览器原生保护。

文件访问失效会取消 IO；同一空间编辑文字暂留页面供复制，恢复访问后覆盖仍需重新读取。预览直接下载，原生客户端分享复用下载文件副本，不新增公开链接或分享服务。完整 Client 传输状态、错误码和缓存边界见 [Android 对接说明](android-platform-integration.md#远程工作区与文件客户端)。

未决 Activation 先用已存 descriptor/hash 匹配 ACK，不因之后的凭据健康观测丢失收敛入口。仅 APPLYING/UNKNOWN 的原控制重放可接受同一 key 的失效健康标记，授权、状态、空间、权限、修订和 SecretRef 仍精确匹配；COMPLETED 恢复和新编译不适用。完成旧操作后保留失效诊断，下一控制阶段撤除绑定。

## 数据与合同

- `000002_remote_workspace.sql` 增加 WorkspaceService、WorkspaceServiceConfig、AgentSpace、WorkspaceOperation、WorkspaceAudit；`000003_workspace_usage_target.sql` 保留旧归属，增加互斥 workspaceTarget。既有 `000001` 不变。
- 旧 upstream 归属保持原字段和序列化；新分支使用 `targetVersion: 2` 与 workspaceServiceId、agentSpaceId、remoteUsername、bindingRevision，仅用于 MCP。预算准入、spool、结算和 Admin 历史详情携带同一固定归属，不引用可变空间表补写历史。
- migration 003 重建表时保留所有原列、索引和引用；结束前完整 foreign_key_check。测试覆盖已有请求、结算引用、原始字节/哈希的升级及重复执行。
- `000006_workspace_webdav.sql` 仅追加 DAV 身份、文件授权意图、独立权限修订、操作修订和基础统计字段；根据已知旧操作/确认状态回填 AUTHORIZED/REVOKED/UNCONFIRMED，不默认重新授权不明旧 key。既有活动服务由 reconcile 补一次 DAV 草稿，不重写历史 Release、Snapshot、Secret 或自动发布管理员草稿。
- `WorkspaceProjection` 的 `schemaVersion: 1` 是独立 Client 控制接口，必填 `serviceState: NOT_CONFIGURED | DISABLED | ENABLED` 区分企业配置意图；`state` 表示用户生命周期，`filesAvailable`/`mcpAvailable` 各自表示实际准入。ENABLED 不等于远端健康。Android generated 导出和 Client 投影样例已同步，不代表 Android 消费端 UI 或真实设备传输完成。未发布的工作区合同直接更新，不新增兼容探测/回退分支；Snapshot 保持 v5。
- 共享历史 Snapshot v4/v5 的发布字节、hash、republish 保持原协议；不在旧实体中填造假的 Upstream。

## 验证和发布边界

执行命令、私有测试配置与真实服务/浏览器分层统一见[测试说明](testing.md#远程工作区专项验证)。历史场景、构建/镜像定位及未确认的间歇大文件传输风险统一见[证据索引](s0-execution-progress.md)。当前脚本通过不等于生产、Android 或完整 S1 验收。部署与恢复仍遵循 Core 数据库备份和 Agent Space 独立发布流程。

### 文件 HTTP 状态与 DAV 完成语义

Core 文件接口保留标准条件/锁定状态：`file_version_conflict` 为 412，`file_locked` 为 423，普通文件冲突及空间不匹配仍为 409。业务 Problem code 与保留草稿、禁止自动重放的恢复语义不变；旧部署曾将前两项统一为 409，消费者按稳定 code 识别期间仍可保留编辑。416 仅输出经过校验的 `Content-Range: bytes */N`，缺失或非法的上游长度不猜测。Hub Files adapter 的上游 DAV 401/403 仍映射工作区凭据不可用，不能冒充 Core 登录失效；平台标准 DAV 入口的普通 403 则原样透传。

DAV adapter 按请求方法检查完成状态；202、非 GET 的 206 或写入的 304 等不能证明文件写入完成，沿已有 `workspace_result_unknown` 返回并要求核实。PUT 的 200/201/204、MKCOL 的 201、DELETE 的 200/204/207、COPY/MOVE 的 201/204/207 分别处理；207 继续解析逐项失败，不伪装为整个目录操作成功。
