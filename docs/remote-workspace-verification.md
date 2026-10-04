# 远程工作区 Core / Admin 验收记录

最近验证日期：2026-09-30。范围为当前 Core / Relay / Admin 候选、隔离 Agent Space 实际管理/MCP/VM/WebDAV 链路和浏览器管理操作；不是生产部署或完整 S1 验收。当前操作说明见 [实现参考](remote-workspace-implementation.md)。

## 配置流程简化验收（2026-09-30）

基于 Core `9c985d2` 和架构 `550a00d`，删除 Admin 的版本能力确认、配置 `releaseIdentity` 字段及固定第三方提交号门禁；工具地址折叠区只呈现可选 MCP 地址。连接检查、地址迁移时的空间核对和指定 DAV token 响应校验仍由原流程执行。测试脚本中的源码/镜像身份仅用于验收记录。

先验证无提交号配置保存和无确认项界面测试失败，再修订实现及生成类型。全量 Go 测试、Admin 40 文件 / 244 项测试、类型检查、生产构建、格式检查通过。以同一隔离 Agent Space 镜像完整重跑 19 组联调通过；实际浏览器修改连接名称、保存配置并等待启用完成，确认不再出现版本声明。

证据在 `.artifacts/workspace-config-simplify/` 的 `evidence.json`、`source-inputs.json`、`browser-evidence.json` 和 `admin-config.png`：917 个源码输入摘要为 `sha256:6381319fa9c5897fe9168417a5b7964bcf424fff71dd486c8793b26d28ecdda7`，Admin 构建为 `sha256:ae7bc8c45d6448a43f44ffad8022b88f482f49dfe96b9bc27de1aba9e9a8bd8c`，Admin 合同为 `sha256:37deabd51670919906c18c0c56807ba25c864ad4b49466f9ac38ff4c70d84658`。本次没有 Client/Snapshot/Relay 合同或数据库迁移变更。

## 资源摘要集成验收（2026-09-30）

执行计划见 [资源摘要计划](remote-workspace-integration-plan.md#2026-09-30-资源摘要执行计划)，六项已完成。基于 Core `35e3ac9`、架构 `e6ea18a` 的工作树实现；Agent Space 固定为已提交 `661d20d8bfe1fb7630a879383257e61602fb6df6`，本轮未修改其源码。其既有 `docker/build.sh` 模式差异原样保留。

| 身份 | 本次实际验证 |
|---|---|
| Agent Space 构建输入 | `sha256:8429f3398403d5ebaa527bc1243035bb6b1e4cd9fa654b9dd30f7b1ea2ab6ace` |
| 实际新建 Docker 镜像 | `sha256:a427de819c17b13db57078bec5d9e79fb8416cb0cea2824d3dc5cb9fc966e7c6`；镜像 label 对应上述提交和输入 |
| Core 输入 | 917 个文件，`sha256:9a8ff5ae960369280288187c1c8c3bdc8d02dfd4b34063fc54a2c105098ece53` |
| Hub / Relay | `sha256:0449bed26d921c7306ddddd73ca1692a48f09ac845e6d7726fa9d23997b519f0` / `sha256:786de779c3e16a4d1046b7d2f2a1b4c3e86aee4a449f6cd0dfdb4d3b88b944d1` |
| 最终 Admin 构建 | `sha256:16651200b32d32689460be36332410fa2422647dcdb23b3b3eaf7daf68e786f2` |
| 最终 Admin 合同 | `sha256:8054cbb32a64f175e030b5519c4990b315ec68cde515edce14ae0b18da9003c6`；Client/Relay/Usage 合同未变 |

环境为独立容器/卷 `measix-resources-20260930`，管理/MCP `127.0.0.1:20932`、DAV `127.0.0.1:20933`。没有改动旧联调容器或生产。测试凭据仅在忽略目录中；可复用命令为 `node scripts/workspace-integration.mjs --config <本机私有配置> --output <证据目录> [--keep-ui]`，配置结构见脚本头部。

- Agent Space：本轮重跑资源单元 9 项、管理 API 7 项，全部通过；其提交附带的完整 218 项真实 VM 记录另作独立证据，不冒充本轮完整重跑。
- Core：全量 `go test ./... -count=1`、`go vet ./...`、合同测试和格式检查通过。资源专项覆盖管理员撤销、空间/绑定/配置在途变化、停用后可读、有效零值、部分失败与诊断脱敏。
- Admin：40 个文件、245 项测试通过；类型检查和生产构建通过。专项覆盖历史/错误/零值、空间切换迟到响应、隐藏页面与卸载停止轮询。容量修复先观察到测试中 24 KiB 被显示为 0.0 MiB，再统一格式后通过。
- 工具与生成：41 项 tooling 通过；417 个生成输入/产物重复生成无变化。审查修复新增 YAML anchor 与既有名称冲突，修复前后生成类型不变，最终合同重新验证并更新基线。
- 实际链路：**19 组通过**。包括重复查看未创建 VM 不启动、管理员鉴权/空间隔离、未发布 MCP 时磁盘/客户机内存正常、64 MiB 文件占用增长、停止后保留历史时间、服务关闭后可查、同用户删除重建拒绝旧空间 ID/清空历史；并回归原 MCP/DAV/Client 文件条件、撤销、恢复、重启和删除。
- 实际浏览器：内置浏览器默认 1265×712 与 390×844；两个入口均查看，实际浏览文件、新建 UTF-8 文本、断开、恢复、刷新。验证未创建/运行/停止、历史磁盘、无内存样本、容器暂停导致的不可达与恢复。窄屏无横向溢出，临时视口已复原。资源查询未干扰编辑或偷偷恢复 DAV 授权。

提交前审查修复两项问题，均先取得 Red 再验证 Green：单项指标或配置的 JSON 类型错误不再导致整份摘要失败；保存配置时不沿用其他版本的确认，必须重新确认并保存当前支持版本。另修正计划正文沿用旧远端版本的表述。修复后全量 Go、前端、vet、类型、构建与格式检查通过，并以新建测试空间完整重跑 19 组实际联调。

最终证据在 `.artifacts/resources-review-final/`：`evidence.json`、`browser-evidence.json`、`source-inputs.json` 对应上表的同一源码、后端、页面构建与合同。最终页面重新检查了冷空间、浏览文件启动、真实磁盘和内存、手动刷新以及宽窄布局；截图为 `admin-wide.png`、`admin-narrow.png`。此前完整生命周期和不可达界面操作证据保留在 `.artifacts/resources-integration/` 与 `.artifacts/resources-final/`。

审查重跑中的一次 64 MiB PUT 收到远端 502，Hub 已传输 34,996,224 字节，正确返回 `workspace_result_unknown`，没有自动重放原写入。失败数据库和日志保留在 `.artifacts/resources-review/`。另建新测试空间后完整链路通过；这不证明 Agent Space/SDK 偶发传输失败的底层根因已修复，本轮未修改独立项目。

剩余边界：`verify:preview-contract` 的 Core 合同基线一致，但独立 Portal 的 `generated.ts`、`client-feed.schemas.json` 仍有既有 source hash 未同步；本轮未修改 Portal 或 Android。没有新增 CPU 使用率、历史监控、计费、资源配额写入或生产部署。

## 首次集成验收（历史记录）

下列候选、构建和 11 组结果属于此前已提交的首次集成，不能代替下文最新修订的验证。

### 候选与环境

| 项目 | 固定身份 |
|---|---|
| Core 基线 | `9f377cf73558ced8c8b1c349f199ffa7f83b2d5c` + 本次提交中的实现（提交前构建验证） |
| 架构基线 | `955dae1cc43ed72f0f3e4e0b38bc1fe0144720a4` + 本次合同文档提交 |
| Core 源输入摘要 | `sha256:46c5070c66926f1cadf20cd4fc18ffc7a8c71e5fd717a7f8cbc9157b2b97a85e`（901 个源码、合同、生成物和工具输入） |
| Hub | `sha256:4850237b3ba74afd18ed069b602dfe64e155f4072cf8f065323b1b115c4ace30` |
| Relay | `sha256:2183ed7667868512befd704bf905ae26c4f349b5c10fe4ef111cd0f07f6188b2` |
| Admin 生产构建 | `sha256:317d0d238b3548d6d17022aee0c6699f7f1b4d52b84d8384b24843c0051831a7` |
| Agent Space release | `3ea01c167fb263f8ef2467b5fe3103353f9a5ddc` |
| 实际 Docker 镜像 | `sha256:150b71a5a19d5db42fcee85fa4945670af7cbd65ce69c7869f14fc20131f4216` |
| WebDAV helper SHA-256 | `da8af8efbf5fcdd44c93cda16b5867b63419495e710e95e1d0a38909a75553ae` |
| Static HTTP helper SHA-256 | `2990b51bbc4688a6c56266885cdf247de1c4fc9b3fe59dcded857eb1c4556f32` |
| 本地工具 | Go 1.26.4 windows/amd64、Node 22.17.0、Quasar 2.23.3、Vitest 4.1.0 |
| 浏览器 | Codex 内置浏览器；测试桌面 1280×900、窄屏 390×844；浏览器接口未报告独立内核版本 |

镜像 ID 来自运行容器 inspect，两个 helper 的摘要在该容器中实际计算；Agent Space Git 工作树由 WSL Git 核对为空，本轮未改源码。容器和持久卷均为独立测试环境。既有 Agent Space 207 项测试是其独立发布证据，本轮没有冒充重新执行。

源输入清单保存在 `.artifacts/workspace-git-review-final/source-inputs.json`：按路径排序，对 UTF-8/LF 规范化内容计算 SHA-256，再对 `path NUL digest NUL` 串计算总摘要；不包含凭据、测试数据库、日志或本文。精确构建及合同摘要另由联调脚本写入同目录的 `evidence.json`。这些忽略目录只作本机诊断，长期结论和执行命令记录于本文。

### 执行结果

| 检查 | 结果 |
|---|---|
| `cd backend; go test ./... -count=1` | 全量通过，包含 SQLite 迁移/备份恢复、管理响应未知、身份校验、控制撤销、历史事件与文件条件/租约异常 |
| `go vet ./...`、`npm run fmt:check` | 通过 |
| `pnpm -C console test` | 37 个文件、232 项通过 |
| `pnpm -C console typecheck`、`pnpm -C console build` | 通过 |
| `npm run test:tooling` | 41 项通过 |
| `npm run generate` 重复生成 | 415 个生成相关文件在 LF 规范化摘要下无变化；包括 Ent、Go/TS wire 和 Android 导出 |
| 工作树差异检查 | staged/unstaged `git diff --check` 通过 |
| 真实服务脚本 | 以下 11 组全部通过，最后完成于 2026-09-29T03:46:38.788Z |

复现命令：

```powershell
pnpm -C console build
node scripts/workspace-integration.mjs --config .artifacts/workspace-integration/acceptance-config.json --output .artifacts/workspace-git-review-final
```

配置文件结构和隔离要求见实现参考；不提交凭据文件。每次使用全新 Core 数据库和独立用户 UUID，成功后清理该轮远端测试空间并停止其 Hub/Relay。

1. **先空间、后工具**：未发布 MCP 即创建两个空间，实际 PUT/GET 文件；服务配置保存不修改能力草稿。
2. 显式添加单个 MCP 草稿定义，使用既有校验/发布/Activation，已开通用户直接获得工具入口。
3. 真实 Relay MCP 与 DAV 读写同一原空间；两个用户隔离，跨用户 session 和伪造身份头不能访问；共享 Snapshot 字节相同。
4. MCP 预算 0 次时返回 429，恢复额度后继续验收。
5. 64 MiB 流式上传/下载摘要一致，HEAD、Range 206、ETag 409/304、无效 Range 416 符合预期。
6. 慢速覆盖上传中途取消，原文件完整保留。
7. 外部 DAV Basic/Bearer 真实请求；断开撤销旧凭据，恢复原空间不会自动恢复 DAV，显式签发后恢复文件。
8. 服务关闭后重新启用恢复原 CONNECTED 意图，不复活手动断开的用户，不自动签发 DAV。
9. 相同数据库重启 Hub/Relay，保留原空间、凭据和文件。
10. 实际准入和结算记录保留不可变 workspaceTarget，未冒用 upstreamId。
11. 远端删除完成后本地绑定回到 UNPROVISIONED；202 不当作已删除。

### 实际 Admin 审查

使用 `--ui-only --output .artifacts/workspace-admin-review` 启动全新未配置环境，实际登录、输入测试连接配置并逐项操作；不是只检查 DOM 模板或以自动化 API 脚本代替网页操作。首轮完整审查地址为 `http://127.0.0.1:56990/admin/remote-workspaces`，临时登录资料仅保存在该目录的私有 `ui-env.json`。

- 菜单、路由、页面文件、store、接口、实体和字段改用远程工作区专门语义。通用 ToolIntegration、Android integration 导出和普通集成测试名称保持原义。
- 实际确认开关不立即提交；“保存配置”才启用；关闭要求保存并明确确认。服务器实际状态与待保存开关分开。
- 实际创建用户、未发布 MCP 时开通空间，建中文目录，上传并打开 Markdown、UTF-8 文本、PDF 和 PNG。Markdown 脚本按文本显示、远程图片不加载；PDF 为真实 worker/canvas 预览。
- 再从工作区跳转 MCP 分区，显式添加、审查并发布；关闭服务后，无空间用户隐藏页签，已有空间保留核查/删除，MCP 定义保留且提示不可用。
- 再启用服务，原空间继续使用、文件仍在；显式重新签发 DAV 后文件入口恢复。MCP 凭据维护从 WebDAV 弹窗移到独立区域，避免混淆。
- 实际复制、重命名、下载后删除本次临时副本；下载 SHA-256 为 `7d565fc26840c38484457617c1cfca8beb57255a19b407b684f4b59c83a2bae6`，与上传文件一致。
- 修复换选上传文件后沿用旧覆盖确认的问题；回归测试能拒绝旧行为，最终网页实际核对勾选状态从 true 变为 false。文件覆盖读取目标版本期间锁住重复提交。
- 桌面与窄屏检查：窄屏内容宽和可视宽均为 375 px，无横向溢出；测试后恢复默认 viewport。最终截图保存在 `.artifacts/workspace-admin-review/remote-workspaces-desktop.png`、`remote-workspaces-narrow.png` 和 `remote-workspaces-overview.png`。

### 提交前复审与修复

对 Core 全部既有变更、生成输入和架构文档逐项复核，并新增以下回归验证：

- 启用前检查失败时恢复原来的启用/停用状态；Relay 控制确认失败保留原绑定并沿原 Activation 重试。
- 缺少 DAV 地址时，命令层拒绝签发 Token，Admin 明确提示先配置地址；地址变更未确认仍指向原服务前禁止保存。
- 创建结果未知后用户被停用或删除，允许核实原账号及空间后只继续清理；不会借接管恢复权限。管理凭据失效支持带处理依据更换当前操作的 Secret 版本引用。
- 上传覆盖确认绑定到所选文件、目标目录及目标 ETag，任一变化都要求重新确认。
- DAV 目录与多状态响应正文增加空闲超时；确定性测试复现原先阻塞并验证及时释放文件操作名额。
- 修正新增 OpenAPI 引用上的无效 description 同级字段，重跑全量合同验证；生成相关 415 个文件重复生成无漂移。

在 `http://127.0.0.1:55288/admin/remote-workspaces` 的独立复审环境实际移除并保存 DAV 地址，确认用户工作区显示前置提示且隐藏 Token 入口，再恢复地址并保存，确认真实文件列表可用。浏览器实际发现并复验“地址确认前保存按钮禁用”的修复。最新 Admin 生产构建已重载；截图为 `.artifacts/workspace-git-review-final/admin-reviewed.png`，缺少 DAV 时的提示截图为 `.artifacts/workspace-git-review-rerun/missing-dav-guidance.png`。本节为对首轮完整网页操作的补充，故障恢复路径由确定性后端/组件测试验证，没有声称在网页中注入全部远端故障。

真实联调首次复审运行在 64 MiB 上传时返回一次 503，前四组已通过；当时未记录错误响应正文，根因未确认。为后续诊断，脚本增加失败响应与私有测试环境记录；随后一次完整重跑通过，补上 DAV 元数据超时及地址确认修复后的最终构建再次通过全部 11 组。通过重跑不作为首次 503 根因已修复的证据。

## 文件客户端合同与编辑能力修订（当前）

Core 基线 `d7f310a`、架构基线 `016e6fc` 加本次提交中的修订。执行范围为服务状态、固定空间请求、完整传输/错误合同、Admin 文本编辑/预览下载及 Android 对接资料；Snapshot v5 不变，没有新增工作区旧版兼容分支。Android、Portal 与 Agent Space 源码未修改。

| 固定输入 | 当前验证身份 |
|---|---|
| Core 源输入 | `sha256:549635bcd3ad37469a99073032018f8397fecbc93eba9480e5d6222cff41555c`（910 个输入；清单位于 `.artifacts/workspace-review-clean/source-inputs.json`） |
| Hub | `sha256:4e5f1b926bb11850224faa9e429cb87dfd9bfb58004c9a30c4f50bd7b54e0e31` |
| Relay | `sha256:20c08a7dbf7fd9e337295b2eef47545580aa5d448e52703ae403f3456ba484f7` |
| Admin 生产构建 | `sha256:8c1074dfb20b0f08f93352d0f2d1490ab22af1e943eede52ec8d83bad0e22b8d` |
| Admin OpenAPI | `sha256:879977e10a41f295e9c75803b8c61a8e4eecbda93fdaba43acc290bc19661ea5` |
| Client OpenAPI | `sha256:55930f4ddc6aa790537f37797f3f4f8f180d431569c8daf9022295d4b9bf90d7` |
| Agent Space | 与首次集成相同的固定 release/镜像；最终使用新容器 `measix-workspace-review-20260929` 和独立卷 `measix-workspace-review-initialized-20260929`，未改其源码 |

- `go test ./... -count=1`、`go vet ./...`、`npm run fmt:check` 全部通过。
- 前端 39 个测试文件、240 项通过；typecheck 和生产构建通过。新增回归覆盖服务状态、固定空间 ID、错误分类、UTF-8/BOM/换行、拒绝混用/重复条件头、条件保存、冲突/未知结果保留草稿与阻止重放、另存/退出确认、访问恢复后的请求生命周期。
- tooling 41 项通过；重复 `npm run generate` 后 417 个生成相关文件规范化摘要无漂移；Admin/Client 投影样例校验通过。`git diff --check` 通过。
- 最终真实服务联调 13 组全部通过，完成时间 `2026-09-29T15:36:43.064Z`。在首次 11 组基础上，增加未开通用户三种服务状态，以及原生 Client Bearer 文件 API 的 UTF-8/BOM/CRLF 字节一致、条件编辑、旧 ETag 拒绝、缺失/错误空间 ID 拒绝。其余管理、MCP、双用户隔离、64 MiB、取消、撤销/恢复、重启、用量和删除链路全部重新执行。

复现最终联调：

```powershell
node scripts/workspace-integration.mjs --config .artifacts/workspace-review-service.json --output .artifacts/workspace-review-clean
```

本轮条件头修复前的构建首跑在大文件 PUT 约 50,855,936 字节时返回 `503 workspace_result_unknown`（前六组通过），证据保存在 `.artifacts/workspace-files-final/`。审计 outcome 为 UNKNOWN，未自动重放写入。未发现可确认的根因；以全新库/新空间完整重跑通过不代表该间歇传输问题已修复。前一轮相似 503 的历史说明仍保留。生产验收前应继续定位这一传输稳定性风险。

提交复审修复了两项可复现问题：UTF-8 BOM 后的正文 U+FEFF 被重复剥离，以及不可用空间的旧目标请求未按合同返回 409；均先观察失败测试再修复。上传计数改用原子读写，避免远端提前响应时传输 goroutine 与诊断/审计读取竞争；不改变流式传输或字节归属。未知写入增加不含路径、URL、原始错误及秘密的结构化诊断。

复审在旧测试容器连续遇到文件首次访问 503，新增日志确认 `remoteCode=file_service_unavailable`、`remoteStatus=503`、`requestCanceled=false`、`leaseCanceled=false`；同一时刻 Agent Space 日志记录 VM 启动 `insert run` 的 SQLite 外键失败。两次失败证据保留在 `.artifacts/workspace-review-push-final/` 和 `.artifacts/workspace-review-approved/`。未修改远端源码或数据库，未重放未知写入；按独立服务初始化流程另建同镜像、相同配置（仅端口变化）的全新数据卷和容器后，最终 13 组联调全部通过。此证据定位了本轮失败边界，不能证明此前大文件中断同源，也不能宣称旧远端环境问题已修复。

此前功能实现阶段的实际网页使用 `http://127.0.0.1:51297/admin/remote-workspaces` 的隔离生产 SPA，完成中文 Markdown 新建、预览、原生下载、编辑、关闭时继续编辑、双页面竞争保存和冲突后另存。旧页面保存返回 409，原草稿保留；另存副本与另一页面保存的原文件分别核实，未发生覆盖。实际下载正文与创建文本一致。该轮构建重载后再次核实原文件版本和另存副本；浏览器未记录控制台错误。

该轮 390×844 窄屏编辑器宽度/scrollWidth 均 327px，文档宽 375px，无横向溢出；桌面和窄屏截图分别为 `.artifacts/workspace-file-edit-review/preview-final.png`、`editor-narrow-final.png`，并发冲突截图为 `editor-conflict.png`。测试后恢复默认 viewport。本次提交复审没有改变布局，BOM 修复由新增逐字节往返测试覆盖。访问失效/恢复及未知写入由确定性组件测试验证，未冒充实际网页故障注入。

## 发布边界与未执行项

2026-10-04 本机 Device Demo 恢复验证：补齐未知 DAV 写入的“核实后断开”出口，后端与页面测试分别先复现阻断再通过；workspace、agentspace、httpapi、runtimecontrol 测试、Console 245 项测试、typecheck/build 通过。保留 SQLite 在线备份后更新本机运行二进制，通过正式 API 断开、修正同机 DAV origin、应用配置、恢复原空间并显式签发 DAV；原空间 ID 不变，文件列表实际返回 200。内置 Chromium 登录管理台确认 MCP/文件均可用、资源摘要正常、文件浏览成功且无待处理操作。未直接修改业务数据库。备份与结果位于忽略目录 `.data/device-real/recovery-dav-20261004/`；这是本机开发实例恢复，不代表生产或手机直连 DAV 验收。

`npm run verify:preview-contract` **未通过跨仓库检查**：Core 自身基线已同步，但 Portal generated.ts/client-feed 的 source hash，以及 Android client fixture、manifest、PlatformWire 与 Portal 副本尚未同步。没有因此修改 Portal/Android 消费端，或把生成导出当作原生客户端已完成。部署整套 Preview 前必须在各消费仓库完成同步及其对应验证。

本轮没有生产部署、真实 Android 工作区文件 UI/旧 APK MCP 验收、任意第三方 DAV 桌面客户端资格认证、生产规模负载/内存测量，也不覆盖完整 S1 compute/storage 计量。外部 DAV 通过真实 Basic/Bearer HTTP 客户端验证；磁盘不足等远端故障边界由确定性测试/远端发布合同分层覆盖，不能声称对所有真实 VM 故障做了注入。迁移/备份验证在隔离数据上完成，不对唯一生产副本做恢复实验。

未知管理写入仍需管理员核实旧请求结束后明确继续；这是现有远端合同边界，不通过自动重放或修改 Agent Space 绕过。首次集成代码已提交；本轮文件客户端修订及复审修复随代码提交并推送；没有部署生产。
