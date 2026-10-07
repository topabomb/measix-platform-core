# API、Snapshot 与兼容实现参考

本文维护 Core 可执行合同、编译和生成机制。语义权威为同级架构仓库的 [Control Protocol](../../measix-architecture/docs/10-runtime-foundation/s0/measix-s0-control-protocol.md)及[阶段阅读清单](../../measix-architecture/docs/measix-stage-document-index.md)所列合同。固定发布与历史验证见[状态和证据索引](s0-execution-progress.md)。

## 合同与版本来源

| 权威源 | 用途与消费者 |
| --- | --- |
| `api/admin/admin.openapi.yaml` | Admin HTTP 与生成 TypeScript；仅 Admin 认证 |
| `api/client/client-control.openapi.yaml` | Android Client Control、Enrollment、Portal Session/Feed/本人用量与文件投影 |
| `api/internal/relay-control.openapi.yaml` | 私有 Hub → Relay 配置与控制 |
| `api/internal/usage-ingest.openapi.yaml` | 私有 Relay → Hub 用量投递与预算准入 |
| `api/portal/portal-contract.openapi.json` | Bridge v3 bootstrap/request/response schema；空 paths 有意保留，不创建 Hub HTTP 接口 |
| `api/protocol-baseline.json` | 当前候选合同身份及四份 HTTP OpenAPI 摘要 |
| `api/fixtures/` | 唯一共享向量与 canonical fixtures |
| `api/generated/android/` | 固定的 Client/Portal 导出与摘要；由源生成，不是原生实现 |

四份 HTTP 文档使用 OpenAPI 3.0.3，按认证和依赖边界分开。Admin/Android 不得生成或调用 Relay internal API。S0.3 的 `api/internal/gateway-control.openapi.yaml` 当前不存在。

当前 Discovery API v1、Enrollment format v1、Bridge v3；Snapshot 新编制为 v5，保留已发布 v4。Relay 工作区控制与 usage target 为 v2，独立 ClientWorkspace 投影为 v1。协议身份、managedGeneration、数据库 migration 和 App 产品版本分别由各 owner 判断。Core 总合同身份唯一来自 `api/protocol-baseline.json`：主合同、明确支持集合和语义基准 Core；Client/Portal manifest 均生成同一身份及 `baselineHash`。具体版本、发行检查及 Android 待调整细则见[发布文档](release.md)。

`node scripts/verify-preview-contract.mjs --mode core` 校验 Core 基线及导出身份；联合和独立发行进一步校验固定消费者材料与证据，命令见[发布文档](release.md)。材料 hash 相同不能替代实际消费验证。

## Snapshot 编译与历史数据

`backend/internal/hub/capability/` 拥有 Draft 校验、canonical compiler、版本 adapter 和发布。Admin Save → Validate → Preview → Publish 使用同一 Draft 和编译链；`DraftPreviewResponse.snapshotSchemaVersion` 来自编译结果，`Release.snapshotSchemaVersion` 来自不可变发布内容，均为只读诊断。

### Starter v5 开场

- Draft 的开场保存在原 `ManagedDraft.content_json` 聚合中，以 expectedRevision/CAS 防覆盖；不建立第二张开场表或保存 API。
- 新草稿编译（`SnapshotInput.SchemaVersion == 0`）仅以 System 的空白判定是否继承所绑定助手，非空文本逐字保留。v5 发布固化有效 System 与有序背景；执行时不再从当前助手补值。
- 旧草稿缺少 opening 可继续读取、编辑和保存；Validate/Preview/Publish 拒绝缺少开场，包括 disabled Starter。只有作者明确初始化才能补齐，GET 不写入业务数据。missing、null 和有效空值按各自 schema 校验。
- `PublishedContent` 从源 Snapshot 恢复开场。Republish v4 产生新 v4 Release/generation，Republish v5 产生新 v5；不把历史 v4 隐式升级为 v5，不改源 bytes/hash。
- v4 使用独立 DTO，Starter 仍是其原有预填合同；v5 required `openingSnapshot`、工具许可和助手 binding 由各自 schema 验证。v5 的旧 `mcpServerIds`、`description` 扩展不能替代现行必填字段。
- 下载返回已存 Release bytes；ETag/304 与完整缓存证明遵循原合同。canonical 资源集合按稳定 ID 排序，开场背景保留作者顺序；Draft 和发布 Snapshot 的 4 MiB 上限分别按实际 UTF-8 字节检查。

Admin 交互见[Console 实现](admin-console-implementation.md)，工具许可与 Tool JCS 见[Direct MCP](direct-mcp-tool-governance.md)，原生映射见[Android 消费参考](android-platform-integration.md)。

### 资源与策略投影

五个策略开关是 required boolean。十项默认值独立可选：助手、对话、快速、标题、附件检查、建议、上下文压缩、文生图、TTS、ASR；省略表示未设置，不能互相继承或按数组顺序推断。六个模型默认引用必须启用，附件检查还要求 IMAGE 输入。`ManagedPolicy` 的 Admin/Client 已知字段含义一致，但读写扩展规则不同。

`ModelDefinition.publishedModelKey` 为空时使用真实 upstreamModelKey。Client v4/v5 的既有 `upstreamModelKey` 字段携带有效公开 selector；私有 Relay Control 同时携带两种 selector/runtime path。四种 LLM 只映射对应 JSON model 或 Google 精确 models 路径段，其余 provider payload/query 保持原样。Relay 保留缺少映射的已应用旧控制状态 pass-through 恢复路径。

预算模板、assignment/revision、Secret、Upstream 私有地址和内部 runtimeRoute 不进入 Client Snapshot。动态余额和工作区连接状态通过独立 API 投影，不推进 managedGeneration。具体准入/结算与预算可见性见[用量额度](usage-budget.md)，工作区空间绑定、文件错误和迁移见[工作区参考](remote-workspace-implementation.md)。

## 响应扩展与校验

遵循 Control Protocol §10.10.3：在已支持版本内递归忽略 Client/Portal/Bridge 响应的未知普通字段，同时校验已知类型、required/null、enum、权限、引用、身份/generation 和完整性。Client schema 采用默认可扩展对象语义；共享 Admin 写入 schema 保持封闭。合同测试比较 ManagedPolicy 已知字段，不能要求读写 `additionalProperties` 完全相同。

Core 先选择生成的 v4/v5 DTO，再进入对应 adapter。PublishedContent 的下载校验与 Admin 草稿命令校验分开，共享已知约束。未来版本在 DTO 解析前明确拒绝，不套用当前类型或静默回退。未知错误 code 保持失败，不能推断身份已撤销。

忽略配置扩展不改变现有 v4/v5 descriptor、ETag/body hash 或历史 bytes/hash；接收方不另造原始 JSON hash。新增输出经 compiler 生成新 bytes/hash，须验证实际旧消费者。`MCP Tool contractHash` 覆盖完整原始 Tool JSON 的 JCS，包括 annotations、`_meta` 和扩展，不适用配置 DTO 的丢弃规则。

生产非泄漏必须由 Hub 编译和预算投影独立验证，不能依赖客户端拒绝未知字段。生成 Go DTO 的 unmarshal 不自动证明全部 OpenAPI required/format/enum 约束；contract tests、HTTP decode 与领域校验共同负责。

## 认证与 Portal 边界

以 OpenAPI 和 HTTP 回归测试为精确结构来源，以下是容易混淆的实现边界：

- Enrollment 创建返回 201，deviceName required；Admin 签发默认一小时，可显式选择一分钟至 24 小时。资料 schema 与 HTTP exchange 请求是不同对象。
- Refresh required Idempotency-Key、轮换凭据与 sessionIdleExpiresAt。缺凭据为 `401 unauthenticated`，无效凭据为 `401 invalid_credential`；拒绝保留 `user_disabled`、`device_revoked`、`session_revoked` 的归属，Runtime 不可验证 JWT 为 `401 invalid_session`。
- 过期接入码 `401 enrollment_expired`；已消费一次性码 `409 enrollment_already_used`；安装绑定其他用户 `409 installation_user_conflict`，此失败不消费码。
- 删除用户走 deny-first durable 状态机：原 Client/Runtime/refresh 凭据返回 `401 enterprise_identity_deleted`，只保留不可逆 tombstone。复用用户名创建新 usr_* 身份，不继承旧私有数据，也不移除原 tombstone。
- Portal grant/exchange 由 Hub 管，ticket/cookie 存 digest 并绑定父 Android Session，exchange 原子消费 ticket。只有 Client OpenAPI 中两条 enterprise/updates GET 额外接受 Portal Cookie；其他 Client/Admin/Runtime 认证不变。
- Portal 本人用量与文件权限由 handler/数据库推导 owner；Native Bridge 不提供本机 Feed 数据源或任意 token 通道。Feed HTTP 字段/查询为 camelCase。
- Admin 自助密码修改需要 Cookie Session + CSRF，在同一事务改 Argon2id hash 并撤销该管理员全部 Web Sessions；显式失败为 `invalid_current_password` 与 `password_confirmation_mismatch`，人类错误文本不是兼容键。
- Admin 的他人密码重设和角色命令还需操作员当前密码；`users/{id}:set-role` 携带 `expectedRole`，旧 PUT 的角色变更进入相同领域校验。新 ADMIN 或无密码用户晋升需原子初始化密码；响应 `passwordConfigured` 从已有 hash 派生。角色变更、密码重设与目标 Web Session 撤销在同一事务，Android 身份不变；当前管理员与最后一个可登录管理员保护按 Control Protocol §22.2 执行。请求密码不存入浏览器持久状态、日志或新增数据库字段。
- `DELETE upstreams/{id}` 携带 `expectedConfigRevision`；事务内保护草稿、全部保留 Release 的 binding 引用及在途 Activation。可删除无引用的 ACTIVE/INACTIVE 连接及其配置修订，保留 Secret、用量、Release 原始字节/hash 和终态操作记录。冲突使用 `upstream_in_use`、`activation_in_progress`、`stale_upstream_config_revision`。

Enrollment 原文向量覆盖重复键、UTF-8 上限、origin/expiry/source；固定时钟下允许 RFC3339 小写 t/z、UTC +00:00、至多九位小数、不接受 leap second，输出大写 T/Z。Go 测试使用局部格式 validator，不放宽全局 HTTP 验证。Bridge 向量另验 method/result 关联、URL trust、chunk progression 和生命周期，schema 通过不能替代这些条件。

## Fixtures 与生成入口

`api/fixtures/client-integration/` 来自真实 compiler 的公开投影配方；其中空 bindings、合成 token/ID 不是运营草稿或登录凭据。Enrollment、Portal/Feed、workspace、Problem 各类共享样例只维护在 `api/fixtures/`，覆盖支持版本、正反例、递归响应扩展及 canonical hash。所有材料不含生产凭据或用户数据。

从 Core 根目录运行：

```text
node scripts/checks.mjs generate
node scripts/checks.mjs drift
```

唯一 generate owner 覆盖四组 Go wire、Ent、Client OpenAPI/manifest、Portal Bridge 与 transitive Feed schema、共享 fixtures、Android integration 副本和 Admin TypeScript；它不生成真实 Kotlin 实现。生成配置、版本和 lockfiles 由仓库固定。

仅更新消费者说明时，运行 `node scripts/export-client-integration.mjs` 刷新 `api/generated/android/integration/`；生成副本不得手改。独立导出只包含 Client 所需 schema/fixtures 和说明，架构正文保留在其仓库。Portal 在自己的仓库从固定来源生成 TypeScript/validator 并记录摘要；Android 在其仓库导入、生成并执行原生验证。

合同门禁从 `backend/` 运行：

```text
go test ./internal/contract -count=1
```

该检查覆盖 schema/fixture、独立导出依赖、版本隔离、已知错误和 hash golden；生成 drift、生产 build 与消费验证仍需各自执行。[测试文档](testing.md)维护完整检查入口。

## 合同变更与独立演进

语义变化先经 architecture，随后 OpenAPI → fixtures → generated artifacts → 最小 Red/Green → 受影响组件/真实边界 → 各消费者。保持语义的 schema 补齐仍同步全部派生产物；若新细节可能改变合理客户端的解释，返回架构审查。

每次升级遵循以下步骤：

1. 区分正在生产使用的格式、正式消费者构建与尚未发布候选。未发布 v5 直接修订，不为中间候选建立迁移；保留已发布 v4 的读取、恢复和回归路径。
2. 明确各端真实支持集合和退役条件。集合由实现与消费测试证明，不能从最高版本、App 版本名、hash 相等或 Core 声明推导 Android 支持。
3. 新消费者在配置失败时保全身份、Binding、历史和原 Applied；个人功能、空间导航和退出不依赖企业配置成功。未知版本、坏内容、网络和身份错误分别处理；企业执行仍遵守当前准入。
4. 固定源码/合同/构建，验证受支持格式、原消费者基本控制、未来版本拒绝与上述失败隔离，以及实际 active release/generation/hash。旧 APK 测试固定原 commit/摘要，不修改其 decoder。
5. 先分发需要的新消费能力，再启用新格式。Core 可先升级并保持旧 active Release；明确恢复目标必须在目标 APK 支持集合内。
6. 正式退役经架构规定的支持窗口和数据恢复验证，不因删除旧 decoder 而删除身份/历史或基本控制能力。变更后创建新候选和实际证据，旧报告不覆盖新 head。

Android `0.0.20 / 20` 尚未正式发行，当前候选修订仍使用该目标；不能据历史 commit 或 Debug APK 把它叙述为“已发布旧版”，也不能因此提前升到 21。发行关联、Core 实施状态及 Android 待完成项仅在[发布文档](release.md)维护。
