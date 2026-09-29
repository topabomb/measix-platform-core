# 远程工作区 Core / Admin 验收记录

验证日期：2026-09-29。范围为当前 Core / Relay / Admin 候选、隔离 Agent Space 实际管理/MCP/VM/WebDAV 链路和浏览器管理操作；不是生产部署或完整 S1 验收。当前操作说明见 [实现参考](remote-workspace-implementation.md)。

## 候选与环境

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

## 执行结果

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

## 实际 Admin 审查

使用 `--ui-only --output .artifacts/workspace-admin-review` 启动全新未配置环境，实际登录、输入测试连接配置并逐项操作；不是只检查 DOM 模板或以自动化 API 脚本代替网页操作。首轮完整审查地址为 `http://127.0.0.1:56990/admin/remote-workspaces`，临时登录资料仅保存在该目录的私有 `ui-env.json`。

- 菜单、路由、页面文件、store、接口、实体和字段改用远程工作区专门语义。通用 ToolIntegration、Android integration 导出和普通集成测试名称保持原义。
- 实际确认开关不立即提交；“保存配置”才启用；关闭要求保存并明确确认。服务器实际状态与待保存开关分开。
- 实际创建用户、未发布 MCP 时开通空间，建中文目录，上传并打开 Markdown、UTF-8 文本、PDF 和 PNG。Markdown 脚本按文本显示、远程图片不加载；PDF 为真实 worker/canvas 预览。
- 再从工作区跳转 MCP 分区，显式添加、审查并发布；关闭服务后，无空间用户隐藏页签，已有空间保留核查/删除，MCP 定义保留且提示不可用。
- 再启用服务，原空间继续使用、文件仍在；显式重新签发 DAV 后文件入口恢复。MCP 凭据维护从 WebDAV 弹窗移到独立区域，避免混淆。
- 实际复制、重命名、下载后删除本次临时副本；下载 SHA-256 为 `7d565fc26840c38484457617c1cfca8beb57255a19b407b684f4b59c83a2bae6`，与上传文件一致。
- 修复换选上传文件后沿用旧覆盖确认的问题；回归测试能拒绝旧行为，最终网页实际核对勾选状态从 true 变为 false。文件覆盖读取目标版本期间锁住重复提交。
- 桌面与窄屏检查：窄屏内容宽和可视宽均为 375 px，无横向溢出；测试后恢复默认 viewport。最终截图保存在 `.artifacts/workspace-admin-review/remote-workspaces-desktop.png`、`remote-workspaces-narrow.png` 和 `remote-workspaces-overview.png`。

## 提交前复审与修复

对 Core 全部既有变更、生成输入和架构文档逐项复核，并新增以下回归验证：

- 启用前检查失败时恢复原来的启用/停用状态；Relay 控制确认失败保留原绑定并沿原 Activation 重试。
- 缺少 DAV 地址时，命令层拒绝签发 Token，Admin 明确提示先配置地址；地址变更未确认仍指向原服务前禁止保存。
- 创建结果未知后用户被停用或删除，允许核实原账号及空间后只继续清理；不会借接管恢复权限。管理凭据失效支持带处理依据更换当前操作的 Secret 版本引用。
- 上传覆盖确认绑定到所选文件、目标目录及目标 ETag，任一变化都要求重新确认。
- DAV 目录与多状态响应正文增加空闲超时；确定性测试复现原先阻塞并验证及时释放文件操作名额。
- 修正新增 OpenAPI 引用上的无效 description 同级字段，重跑全量合同验证；生成相关 415 个文件重复生成无漂移。

在 `http://127.0.0.1:55288/admin/remote-workspaces` 的独立复审环境实际移除并保存 DAV 地址，确认用户工作区显示前置提示且隐藏 Token 入口，再恢复地址并保存，确认真实文件列表可用。浏览器实际发现并复验“地址确认前保存按钮禁用”的修复。最新 Admin 生产构建已重载；截图为 `.artifacts/workspace-git-review-final/admin-reviewed.png`，缺少 DAV 时的提示截图为 `.artifacts/workspace-git-review-rerun/missing-dav-guidance.png`。本节为对首轮完整网页操作的补充，故障恢复路径由确定性后端/组件测试验证，没有声称在网页中注入全部远端故障。

真实联调首次复审运行在 64 MiB 上传时返回一次 503，前四组已通过；当时未记录错误响应正文，根因未确认。为后续诊断，脚本增加失败响应与私有测试环境记录；随后一次完整重跑通过，补上 DAV 元数据超时及地址确认修复后的最终构建再次通过全部 11 组。通过重跑不作为首次 503 根因已修复的证据。

## 发布边界与未执行项

`npm run verify:preview-contract` **未通过跨仓库检查**：Core 自身基线已同步，但 Portal generated.ts/client-feed 的 source hash，以及 Android client fixture、manifest、PlatformWire 与 Portal 副本尚未同步。没有因此修改 Portal/Android 消费端，或把生成导出当作原生客户端已完成。部署整套 Preview 前必须在各消费仓库完成同步及其对应验证。

本轮没有生产部署、真实 Android 工作区文件 UI/旧 APK MCP 验收、任意第三方 DAV 桌面客户端资格认证、生产规模负载/内存测量，也不覆盖完整 S1 compute/storage 计量。外部 DAV 通过真实 Basic/Bearer HTTP 客户端验证；磁盘不足等远端故障边界由确定性测试/远端发布合同分层覆盖，不能声称对所有真实 VM 故障做了注入。迁移/备份验证在隔离数据上完成，不对唯一生产副本做恢复实验。

未知管理写入仍需管理员核实旧请求结束后明确继续；这是现有远端合同边界，不通过自动重放或修改 Agent Space 绕过。代码与文档作为本次审查提交；没有推送或部署。
