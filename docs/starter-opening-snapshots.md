# Starter 开场快照：Core 方案、实现与验证

## 1. 目标与范围

核对基线：Core `1b70fcb89e547dbceeb9acf06bf0dfa0bd45c894`；Android `a710efa59420377c764e7c18bb9c01f4361d5f99`。
本次扩展企业 Starter 的编制、发布和消费，保持已有持久数据、已发布配置与 Android v4 功能可用。
本地实施包含 Core、Android，拥有跨端语义的 `measix-architecture` 文档，以及 Portal 必需的派生类型/来源摘要同步；所有改动保留在工作树，不提交或发布生产服务。

Starter 是管理员编排的任务起点，不是已执行过的对话。它包含用户可编辑的起始提示词，以及管理员准备的领域指令和有序背景。
开场快照不包含工具执行记录、模型答复、运行记忆、凭据或未来设备的完整 System。客户端固定规则、工具规则仍由 Android 在 START 组装。

本方案的完成条件是：管理员可在原界面完整编制并发布 v5；旧 v4 数据和重新发布含义保全；Android 使用 Core 权威协议及实际产物完成下载、实例化和详情验证。
生成 DTO 或 Mock 单测通过不能代替这三条用户路径。真实生产 v5 部署、Gateway、物理设备性能及正式阶段封版不在本次范围。

## 2. 当前链路及审查结论

| 领域 | 当前事实 | 调整 |
| --- | --- | --- |
| 草稿 | `ManagedDraft.content_json` 保存完整定义；`GetDraft/PutDraft` 负责读取和 revision CAS | 在 Starter 子对象增加 opening；不拆表、不增加写入口 |
| 发布 | `ManagedRelease.release_content_json`、`snapshot_json`、`snapshot_hash` 是独立不可变发布事实 | 新发布编译 v5；旧行不回填、不重新编码 |
| 编译/预览 | `CompileSnapshot` 是客户端投影和 hash owner；Preview 使用同一编译器 | 新草稿空白 System 解析为所选助手指令；非空覆盖、背景顺序及原文保留，不回写草稿 |
| 下载 | `GetSnapshot` 返回已存 bytes/hash，HTTP 层据此生成 ETag/304 | 保留原路径，读取旧发布不做升级 |
| 重新发布 | `PublishedContent` 从不可变 Snapshot 恢复有效 opening，保留 release content 的其他内容与运行绑定 | 以显式源版本调用 `CompileSnapshot`，跳过草稿继承解析，创建新 release/generation |
| Admin | `ManagedExperienceEditor` 经同一 Draft store 保存、校验、预览、发布 | 助手详情横向标签；Starter 摘要列表进入大号单列对话框，开场按需展开 |
| Android | 消费 Core 导出的 v4/v5 OpenAPI、manifest 与共享样例 | 确定性 adapter 与 device:real 实际供应商分别记录证据，保留独立 Mock 网络场景 |

持久化实际已由 `backend/migrations` 的有序 migration history 管理，启动不自动迁移；旧文档中“只有初始化、可删除旧数据库”的描述不再适合作为本次兼容策略。
本功能不改变关系表结构，不修改 `000001_initial.sql`，也不新增无作用的 migration。备份、隔离恢复与原始发布内容保全仍需验证。

## 3. 版本和兼容决策

### 3.1 唯一版本含义

- Snapshot v4：原 Starter 只预填起始提示词，不含 `openingSnapshot`。
- Snapshot v5：每个 Starter 必须具有完整 `openingSnapshot`，包括 disabled 条目；当前 Snapshot 并未过滤这些定义。
- Discovery/Bootstrap 声明支持 `[4,5]`；新 Draft 的 Preview/Publish 使用 v5。
- 尚未实施的 Gateway 不能继续占用 v5；架构将其预留为 v6，当前不声明或提供。
- HTTP API v1、Portal Bridge v3、接入资料 formatVersion=1 不因此变化。SQLite schema 和 Android manifest 版本也不跟随 Snapshot 版本递增。

没有按请求协商降级 Snapshot 的新 header。目标 generation 的版本属于该不可变发布：旧客户端面对 v5 应明确提示版本不支持，不能由服务器删字段伪造同 hash 的 v4。

### 3.2 四种存量状态

| 状态 | 读取/修改/发布规则 |
| --- | --- |
| 旧草稿缺 opening | 原样读取为未编制；可编辑及 CAS 保存；管理员显式初始化后才能通过 v5 发布校验 |
| 新草稿有 opening | 保存编制值：空串/全空白继承同一草稿中助手当前指令，非空白为独立覆盖；Preview/Publish 固化有效值，不回写草稿 |
| 旧 v4 发布 | 原 bytes/hash/ETag、来源内容、Session/Applied 记录均保留；查询不变更 revision 或 generation |
| v5 发布 | 原样保存完整 opening，后续发布不改写已实例化的客户端聊天 |

缺失不能与 `null` 混为一谈。旧草稿缺字段是明确的未编制状态；显式 null、错误类型、未知 format、重复 ID 或损坏正文结构属于非法内容。
Admin 草稿允许“尚未完成”的定义，因此 opening 在 Admin DTO 中可缺失；Client v5 的 required 不能因这一管理需求而放宽。

`CompileSnapshot` 仅在 `SnapshotInput.SchemaVersion == 0` 的新草稿入口用 `strings.TrimSpace` 判断继承；显式历史版本保留字面 System。`PublishedContent` 同时用于发布差异与历史重新发布，按 Starter/Assistant 身份核对源 Snapshot，缺失、重复或不匹配直接失败，原持久内容不改写。

重新发布历史 v4 时创建新的 v4 release/generation/hash；重新发布 v5 则创建 v5。两者都读取原 release 内容，并由 `PublishedContent` 从源 Snapshot 恢复固化 opening，不借用当前草稿或当前助手。原 release content 中空白可能是编制时的继承意图，不能直接当作最终 System；历史 Snapshot 自身的空串/全空白则按字面保留，不重新继承。
把历史发布自动转成 v5 会把“使用助手当前指令的提示词入口”改变为“固定领域指令的开场”，不符合恢复历史配置的含义。
升级到 v5 应经草稿编制、校验、预览和明确发布。程序升级本身不发布新配置，不注销客户端、不删除缓存、不改已有 generation。

## 4. 精确数据语义

Client v5 与 Admin 使用相同 opening 内容结构，字段的可缺失范围由外层契约决定：

```json
{
  "openingSnapshot": {
    "format": 1,
    "systemPrompt": "用于此任务的领域指令",
    "initialContexts": [
      { "id": "context-1", "title": "处理约定", "content": "已确认的背景原文" }
    ]
  }
}
```

| 字段 | 规则 |
| --- | --- |
| openingSnapshot | v5 Starter 必填、非 null 对象；Admin 缺失表示未编制 |
| format | 必填整数，当前仅 1 |
| systemPrompt | 必填字符串。Admin Draft 空串/全空白表示继承同一草稿的助手指令；非空白覆盖原样保留。Client Snapshot 是编译后的固化值，空值不再解析继承 |
| initialContexts | 必填数组，可为空；顺序是作者意图，不能排序或去重 |
| id | 必填字符串，至少一个非空白字符，同一 opening 内唯一；不要求平台 ID 前缀 |
| title | 必填字符串；Client v5 至少一个非空白字符。Admin 允许保存未完成的空白标题，Validate 精确定位并阻止发布 |
| content | 必填字符串，可为空；不执行模板、不解释 HTML、不改换行或修剪正文 |

保留中文、占位符、反引号和重复正文。字段对象保持严格结构；不通过忽略未知字段或补零值掩盖输入错误。
Go 生成 DTO 的零值不能证明 required/null 合法性：HTTP 解码和 durable 读取必须明确验证这些边界，领域校验提供可定位的业务错误。

最外层 Starter 集合仍按 stable ID canonical 排序；opening 的背景顺序原样进入 descriptor/hash。只改 System、背景内容或顺序应改变 projectionHash 与发布内容差异；只重排资源集合不应改变规范化投影。
Client 最终 Snapshot 编码最大 4 MiB，与 Android 接纳上限一致；草稿写请求独立使用 4 MiB 上限，其他 Admin 请求沿原限制。
两者分别检查实际 UTF-8 JSON 字节，不拿输入大小代替输出大小，也不新增任意的块数/单块字符配额。

## 5. 存储、查询与恢复

### 5.1 为何继续使用现有聚合

Starter 与背景当前只随完整草稿或发布一起查询、校验和提交，没有跨 Starter 搜索背景、独立授权或独立修改事务。
在既有 aggregate 内嵌有序值对象可以保持一个 owner、一次 CAS 和一个发布快照；拆成背景表会增加发布复制和排序事务，而不能改善当前查询。
不新增 revision 表、索引、全局背景注册库或第二个 JSON sidecar。背景 ID 只识别该模板内部的条目，不是共享可变资源的引用。

### 5.2 落盘变化

| 持久内容 | 变化及保全 |
| --- | --- |
| managed_drafts.content_json | 正常保存时增加 opening；缺失保持可见未编制状态，GET 不写回、不推进 revision |
| managed_releases.release_content_json | 保存完整编制定义与运行绑定，空白 System 可保留继承意图；最终有效 opening 由 snapshot_json 提供，旧行永不回填 |
| managed_releases.snapshot_json/hash | 新版本投影包含完整 opening；旧原始编码和 hash 保留 |
| activation、runtime control | 原发布状态机不变；开场内容不进入 Relay 路由或 Secret 描述 |
| Session/Device applied generation/hash | 不变；仍按实际发布身份上报和校验 |
| 备份/恢复 | 沿现有数据库 backup/check/restore owner；同时验证 v4 bytes 和 v5 值对象 |

数据格式兼容不是 SQL migration。数据库 schema 版本不动，原 migration checksum 不变；已有备份恢复后可读旧草稿和发布，再经正常管理流程编制 v5。
损坏数据应保留原错误和可定位身份，不自动清库、补空对象、换用当前助手或降级发布。

## 6. Admin 交互

### 6.1 编制

`ManagedExperienceEditor.vue` 保留外层资源导航与助手列表，助手详情内部使用“基础/指令/记忆/模型与 MCP/常用入口”横向标签。Starter 列表只展示摘要和操作，长文本在 `.app-dialog--lg` 单列对话框编辑；header/footer 固定，`.app-dialog__body` 滚动，开场上下文默认折叠。
对话框的 `editingStarterId` 只定位 `draft.localContent` 中的条目，没有第二业务 store。关闭/完成只清编辑视图身份，保留未保存修改，重开继续；页面原保存、验证、预览和发布生效，不新增保存 API。基础开关仍在原详情，不将全站设置弹窗化。

1. `draft.addStarter` 创建空 System 覆盖与空背景数组；空串及全空白在新草稿编译时继承所选助手当前指令，不复制为隐式覆盖。
2. 旧 Starter 缺 opening 时显示未编制状态，由显式动作初始化为空覆盖与空背景，不在读取时补造。
3. 展开后编辑可选 System 覆盖与有序背景；空白显示继承及只读助手预览，非空覆盖保留完整原文。助手修改不覆盖独立文本或背景。
4. 背景提供标题、正文、新增、上移、下移、删除；ID 自动生成，重排不重建 ID。删除只改本地草稿，不额外弹确认。Starter 列表上移/下移调用 `draft.moveStarter` 调整本助手顺序，不影响其他助手；sortOrder 保留在存储与排序中，不作为数字编辑字段。
5. `useAssistantSystem` 在丢弃非空覆盖时确认，然后清空覆盖恢复继承，保留全部背景；不是将助手当前正文复制为新的固定覆盖。
6. 所有修改进入现有 localContent/dirty 状态；Save 使用原 revision CAS。409 保留未保存内容；即使服务端未返回可选当前 revision，也显示重新加载入口，不虚构版本号，取消重新加载仍保留编辑。

不自动复制 memorySeed；它有独立的披露来源，重复复制会制造过时背景。disabled Starter 仍可编辑，disabled 不是编辑锁。
校验错误打开正确的 Starter 编辑对话框及折叠组，聚焦到具体 System/背景字段，不能让错误藏在收起内容中。

### 6.2 预览和发布

Preview 展示 Hub 同一 compiler 产生的起始提示词和默认折叠的开场内容。正文使用纯文本渲染；完整 System/背景可按需展开，默认不展示 format/hash/ID。
Review 保持原发布摘要与动作。发布成功后仍等待既有 authoritative Activation；没有新开场专用发布接口。

审查已发现原 Review 异步期间仍可编辑，返回后可能发布已保存基线并丢弃未保存内容。
本次统一阻止互相冲突的加载、保存、校验、预览、审查和发布操作，并在结果接纳/发布前核对 revision 与 dirty；不能仅依赖按钮禁用或事后刷新。

## 7. Android 对接

Core OpenAPI 与共享 fixture 替换 Android 自有目标 schema 的权威地位。保留必要的不可变 v4 golden，避免升级后的生成器改写其历史 bytes/hash。
Android 普通构建仍使用仓库内固定导出，不依赖实时 sibling checkout；生成器和测试验证来源摘要、字段必填、版本分支与实际样例一致。

- v4：仍只填 prompt，Draft 不凭空绑定 opening。
- v5：选择时填 prompt 并绑定定义；用户发送前不建 durable 聊天、不自动请求模型。
- 首发：复验当前定义及权限，保存完整来源与有序背景；最终 System 在 START 组装。
- 后续发布：不改变已有聊天 opening；配置变化和工具结果继续沿已实施的上下文规则。
- UI：保留原卡片/菜单/输入框，开场额外信息折叠；详情可核对实际原文。

真实本地跨端流程应由 Core HTTP 提供 discovery、接入、bootstrap、Snapshot v5 与当前发布身份，再由 Android 正式 owner 消费。
Core lane 可连接本地确定性 adapter，也可连接 `device:real` 的实际供应商；临时接入资料和目标 Starter 由对应输入提供。adapter 请求捕获用于确定性 wire 核验，真实供应商响应另行记录，二者不能互作通过证据；本地 device:real 也不等于生产 v5 部署验收。

## 8. 实施顺序与职责

1. 先同步拥有语义的架构文档，消除 v5/Gateway 冲突与过时的数据删除规则。
2. 更新 Admin/Client OpenAPI、共享有效/无效样例、生成 Go/TypeScript/Android 导出。
3. 以最小真实失败测试推进 strict decode、值对象校验、hash、Preview、Publish、Republish、旧数据保全。
4. 实施 Admin 摘要列表/对话框编辑、继承预览、顺序操作和原 Review 竞态修复，补高价值交互与 CAS 测试。
5. Android 接入 Core 生成权威，清理无调用 Mock 契约路径，保留独立用途的确定性测试服务。
6. 执行 Core 全门禁、真实浏览器作者流程、Android 定向及设备协议流程；发现不匹配直接修正对应 owner。
7. 最终检查全部差异、生成幂等、文档事实和两端工作树；不提交、不部署生产。

## 9. 验证矩阵

| 编号 | 必须证明的结果 | 证据层 |
| --- | --- | --- |
| V01 | v5完整定义、缺失/null/错误类型/unknown/format/id/title约束；空值合法 | OpenAPI、HTTP、领域测试 |
| V02 | 新草稿空白继承、非空覆盖逐字保留，不回写编制值；固化正文/顺序/ID与hash/diff一致 | compiler golden |
| V03 | 旧draft读取不写库；显式初始化保存；stale CAS不丢新内容 | SQLite + Admin测试 |
| V04 | 旧release bytes/hash/ETag/304不变；PublishedContent恢复固化opening，v4/v5重新发布保持版本，历史空/空白System不重新继承 | HTTP + RuntimeControl |
| V05 | Preview/Publish共享投影，大小上限检查不产生半发布 | 领域 + HTTP + failure测试 |
| V06 | 旧数据与新opening backup→隔离restore→check→业务读取 | maintenance/SQLite |
| V07 | 新建默认继承、独立覆盖/恢复继承、背景增删重排、Starter排序、disabled编辑、冲突保全 | Vitest |
| V08 | 摘要列表/对话框、关闭重开保留未保存修改、默认折叠、精确错误定位、纯文本预览、Review并发修改保护 | Vitest +浏览器 |
| V09 | 桌面/窄屏标签及对话框编制→页面保存→刷新→预览→发布→实际Snapshot比对 | 真实Hub/Relay/SPA浏览器 |
| V10 | Android采用Core权威导出，v4/v5严格消费与304来源匹配 | 生成校验 + JVM |
| V11 | Core实际v5下载→Android滚动选择目标卡片→首发→Room与原文详情→重开；确定性adapter与实际供应商独立记账 | Android专用模拟器 |
| V12 | 全Go测试/vet、前端typecheck/test/build、工具/contract检查、Android风险匹配门禁 | 当前运行报告 |

不以人工拼写的 PASS、旧 commit 的报告、生成类型存在或测试源码存在代替运行结果。
环境跳过、系统中断、失败后修复重跑和真实模型边界分别记账；合并过时/重复实现的测试时保留上述行为证据。

## 10. 实施及验收记录

实现已落地，以下记录本次工作树的实际运行证据；不是生产部署或正式阶段封版。

### 10.1 实施结果

- Core：单一 v4/v5 compiler、严格 opening 校验、未编制草稿、原发布版本重新发布、4 MiB 双边界与备份恢复保全。
- Admin：摘要列表、上下排序和共享大号编辑对话框；空白独立指令默认继承助手 System，发布固化。保留稳定背景 ID、作者顺序和错误字段定位。修复 Review 异步覆盖风险、无 revision 的 409 恢复入口，以及 Preview/Review 返回后丢失所选助手与标签的问题；320px 说明采用正常文档流。
- Android：删除独立目标 schema 与对应测试，采用 Core OpenAPI/manifest/cases；保留用途独立的 Mock 网络场景。v5 开场沿正式同步、Draft、Conversation、Room、详情 owner 消费。
- Portal：仅同步 Client 派生类型、Feed schema 的来源摘要及生成清单，运行逻辑未扩展。
- SQL migration 无新增，已存在 migration 与 v4 Snapshot golden 均未修改。各仓按领域提交，生成消费端随权威契约同步。

### 10.2 基础门禁

日志基目录为 Core `.artifacts/starter-v5/`；Android 为 `build/reports/core-starter-v5/`。这些是完整构建与设备基线；默认继承和对话框增量的最终复核、测试 APK 身份及未复现失败见 §10.5。

| 门禁 | 当前结果及证据 |
| --- | --- |
| Core 全 Go 测试 | `go test -p 1 ./... -count=1` 通过，`core-full-go.log` |
| Go vet / gofmt | 通过，`core-vet.log` / `node scripts/checks.mjs fmt` |
| 持久化与协议定向 | 六包通过，`backend-final-verified.log`；覆盖 backup→restore→check、v4/v5 原发布重新发布、原 bytes/hash、边界拒绝与零写入 |
| Console 完整测试 | 207 项通过，`console-ui-final-tests.log`；typecheck 与最终 production build 通过 |
| 工具验证 | 35 项通过，`tooling-final.log`；包含旧 Assistant System 重复拼入、instrumentation 假成功与来源漂移反例 |
| 生成幂等 | 374 个文件重生成零变化，`generation-idempotence.json`；跨 Core/Android/Portal baseline CLI 通过 |
| Portal | 94 项测试与 production build 通过，`portal-test.log` / `portal-build.log` |
| 真实本地跨端 | 最终 `core-android-harness-private-input.log` 全流程通过：6 个浏览器用例、实际 Hub/Relay/SQLite、Android opt-in 1 项通过（22.727 秒）；`core-android/verification.json` 核对实际 wire；桌面/320px 与设备原文展开截图均已复查 |
| Android 完整门禁 | `test assembleDebug lintDebug assembleRelease --no-parallel --max-workers=1` 通过（8m 57s），`android-full-fixed.log`；全部模块 2,840 项，12 项既有环境跳过，0 失败；其中 app 2,284 项通过 |
| Android lint | 0 errors、322 warnings、6 hints；未禁用检查或新增 suppress |
| Android 定向 connected | `connectedDebugAndroidTest` 的 Starter 持久化与 v4/v5 入口两项通过，无跳过，`android-connected.log` |

真实跨端使用同一 Admin 作者流程保存/发布的 Snapshot。专用 `emulator-5562 / Codex_Context_V5_API36` 通过公共 Client API 接入并发送 Starter；adapter 捕获确认最终 System 包含 opening 且没有重复追加原 Assistant 指令，背景 ID、顺序、字面内容及起始提示词一致。设备验证 Room 读回和 Activity 重开详情；不将 Activity 重开写成进程/数据库重启。Android 自身仍附加应用固定规则和独立背景，Starter 不占有整个最终 System。

### 10.3 失败、修复与边界

- Core 导出最初遗漏 `x-uniqueProperty: id`，Android strict 测试实际失败后，在 Core 权威 schema 修复并重新生成；未放宽消费端校验。
- 更新共享 Bootstrap 为 `[4,5]` 后，一个“Bootstrap 不再支持下载版本”的 Android 网络反例失去前提。改为在该场景显式返回 `[4]`，保留原拒绝和 Applied 保全断言；定向重新通过。
- 第一轮浏览器在合法 409 未带可选 revision 时找不到重新加载操作。修复 DraftStore 冲突状态并新增 Red/Green 覆盖，真实链路重新通过。未靠修改选择器或重试隐藏产品缺陷。
- 独立审查补齐 verifier 的新来源 header/manifest 检验和原 Assistant System 排除断言。
- 实际截图发现窄屏 hint 与按钮重叠；局部改为正常文档流，增加几何断言。设备截图同步等待布局并滚动显示原文，业务断言不变。
- 全新安装的设备复验暴露测试交接的目录 ownership 错误：adb 创建的外部目录属于 shell。改由 `run-as` 经 stdin 在应用私有 cache 写入权限 600 的临时输入，应用读后删除、harness 最后兜底删除；读取失败和清理失败保留主因及 suppressed。无敏感 probe 已验证中文原文、App UID 读取/删除；整链重新执行。

保留原始失败日志；首次整链成功保存在 `core-android-first-pass/`，目录交接失败保存在 `core-android-input-failure/`。最终生产 Debug APK SHA-256 为 `552213944742bd59ff90926c8ef9b6485c8f47cc2025cd87e82dd02ab9659f2a`，测试 APK 为 `73b0152ad832466bfa21799a4d49eddb3c794621e87329b26bb3422820dd6aba`；测试端最后只调整截图等待和临时输入异常保全，业务代码与完整构建门禁一致。

新增接入资料仅用于隔离测试，读取后删除；不写源码、文档或提交。既有生产 demo `emulator-5560` 保留，未清数据或改企业配置。本节模型为本地确定性 adapter，不宣称生产 v5、真实供应商能力或物理设备验收。该次四仓 diff/编码检查和 32 个变更 Markdown 的真实文件链接检查通过；证据身份记录在 `final-verification.json`。后续实际环境结果见下节。

### 10.4 device:real 与实际 Admin 操作

2026-09-27 实际运行 `npm run device:real`，复用 `.data/device-real`，未清数据库、未重建凭据。首次发布因三个预置 Starter 缺少 opening 失败（`start.log`）；修复为显式 System 和有序背景后启动成功（`start-fixed.log`）。同时修正 validation 的真实 `path` 显示，以及迁移失败不应建议 reset 清库的说明。`real-device-preset.test.mjs` 记录实际 Red/Green：完整 opening、无变化不发布、失败阻断、Secret 引用复用、诊断与非破坏性迁移提示，共 7 项通过，已加入工具门禁。

通过实际浏览器登录本地 Admin，在既有助手中新增“开场快照联调”，修改独立 System、添加两条背景、上移重排、保存、整页刷新、展开服务器 Preview，并经 Review 发布。没有通过接口代写被验收的编制/发布操作。初始提示词始终可见，其他内容默认折叠；320px 页面无横向溢出，`{{unchanged}}` 保持字面原文。修正原“只会预填”的误导说明为预填提示词并设置开场上下文，中英文同步，布局不变。

本次网页发布为第 2 版，release `rel_8a0a4061-6de7-488a-95cd-c47ae26a6911`，hash `sha256:7f1cd54cd2a7f58a98d5b793a9e24ba5b96d5be145fd2662bb2fb2a2a9b267d1`。通过同一网页创建专用成员和一次性接入资料，Android `emulator-5562` 正式同步、报告 Applied、横向滚动选择第三张入口卡片、预填并首发。原测试假设目标卡片已组成，修正为滚动可见 Starter LazyRow 到目标后点击；不匹配隐藏导航抽屉，不改产品布局或成功断言。

实际 DeepSeek 返回 `401 authentication_error`，设备显示原始诊断；本地密钥直接校验同样返回 401。百炼直接探测返回 `403 AccessDenied.Unpurchased`。这两项不是成功模型验收；用户已确认密钥可能过期，将本轮收口范围明确为功能正确性与交互合理性，不再更换凭据或改模型 ID。ACTIVE 不等于供应商凭据有效。成功响应后的完整流程使用确定性 adapter 独立验证，不能以它宣称这些供应商恢复。

本节证据位于 `.artifacts/starter-device-real/`：实际网页截图 `admin-preview-320.png` / `admin-published.png`、实际发布原文 `admin-published-snapshot.json`、启动失败/修复日志、Android 原始 `android-deepseek-auth-failure.log` 与失败截图。两个滚动测试失败轮次分别留档；AVD 名称输出额外 CR 导致的前置检查失败也单独留档，不计入通过结果。一次性资料经应用私有 cache 交接并删除，未进入源码或文档。原生产 demo `emulator-5560` 未动。

### 10.5 默认继承与对话框的最终复核

本节记录默认继承语义与独立对话框落地后的验证，覆盖本节之前记录的界面状态。实际重新运行 `device:real` 加载新后端与生产 SPA，保留数据库和旧发布；启动脚本按其既定行为重建本地验证草稿。通过实际网页新建“默认指令与背景验证”，确认独立指令初始为空、助手指令可展开查看、背景原文不进入起始输入。操作添加背景、列表上移、关闭重开、保存与服务器 Preview；Preview 中的有效 System 等于助手原文，`{{unchanged}}` 未替换。缺失背景标题的真实校验错误能打开对话框并聚焦正确字段。

网页实操与回归共同修复两个问题：Preview/Review 不再卸载工作区，返回后保留选中助手与内部标签，隐藏时使用 inert/disabled/active 隔离；共享对话框尺寸规则不再被 Quasar 的 560px 默认上限覆盖。实测桌面宽度 760px，320px 窄屏无横向溢出，正文独立滚动且完成按钮可达。证据为 `admin-dialog-desktop.png`、`admin-dialog-320.png`、`admin-starter-list.png`。原生确认框在一次浏览器控制中阻塞了工具标签，改用新标签继续实操；取消/确认恢复助手指令并保留背景两条路径由真实 Playwright 浏览器覆盖。

最终代码门禁记录于 `.artifacts/starter-device-real/`：

- `backend-final.log` / `vet-final.log`：全 Go 测试与 vet 通过；继承、历史 v4/v5 空值、Diff/Republish、存储保全反例由真实后端测试覆盖。
- `console-final.log`：209 项通过；typecheck、production build 通过。两个返回位置回归均先复现失败再通过。Portal 94 项通过；工具门禁 39 项通过。
- `generation-idempotence.json`：92 个相关 wire/fixture/导出文件重生成无变化；`contract-final.log` 跨仓来源校验通过，格式检查通过。Android 四类契约定向测试与 Debug/测试 APK 构建通过，见 Android `final-contract-device-build.log`。
- `deterministic-final.log`：生产 SPA、真实 Hub/Relay/SQLite、6 个浏览器用例及 Android 1 项通过（26.665 秒）。浏览器覆盖空白继承、独立覆盖重置取消/确认、背景保全与排序、关闭保留、冲突、预览返回、760px/320px 几何与发布。
- 本轮真正使用的 Snapshot 为 `rel_a16057c6-aab8-456f-a592-abde050f15ec`、generation 2、`sha256:5ad2cd437f25dabc2d85acb6e93d30a80b482828150f8275745b05f0424b387a`；`.artifacts/starter-v5/core-android/verification.json` 与 `adapter-requests.json` 记录其实际 wire。Android 首发、Room 读回、详情原文展开和 Activity 重开通过，截图已复查。

失败边界仍明确保留：一次设备详情持续加载超时，独立静态审查未找到可确定的原因，清理专用测试环境后同一 APK/用例复跑通过；不将它写成已修复的产品缺陷。该失败原始记录为 `deterministic-context-loading-failure.log`，其 semantics/截图保存在带时间戳的 `core-android-*` 留档。另一次未清理设备的重跑被 fresh-unbound 前置条件正确拒绝，未计为业务成功。harness 每轮先归档旧输出目录，防止 adb pull 嵌套旧目录以及旧 verification.json 混入失败轮次。过期 SPA 导致的中断轮次也不计入通过。未增加重试或放宽业务断言。

生产 Debug APK SHA-256 保持 `552213944742bd59ff90926c8ef9b6485c8f47cc2025cd87e82dd02ab9659f2a`，本轮测试 APK 为 `00c4c1194a1a0d0a87df42ce0a5fd9ebe34b7436d277a7aead8e29b4a05513ae`。本轮验证没有部署生产或修改既有生产 demo；供应商成功调用仍不在此次确认的验收范围内。


### 10.6 交付独立审查与完整复验

两名新的独立审查者分别核对 Core 后端/发布保全和 Android/Portal/管理 UI/架构契约；Android 范围包含本次稳定上下文实现的未推送提交，以及后续消费端变更。确认并修正三项问题：

- Review/Preview 的校验错误链接先关闭审查面并恢复编辑工作区，再定位 Starter 字段；busy 期间不接纳导航。两个用例先复现隐藏/inert 错误，再修复通过；不丢草稿或改变布局。
- 删除“空助手 System 可发布”的错误承诺。助手指令原校验继续生效，新增集成反例验证 Validate 的精确诊断与 Stage 拒绝；Preview 是投影能力，不代替发布校验。历史 opening 的空串/空白字面值保全规则不变。
- 统一版本范围：新发布 v5，已发布 v4 保全并可消费；不支持 v1/v2/v3 原型。实施决策与系统/Gateway 测试要求不再互相矛盾。

本轮证据目录为 Core `.artifacts/starter-delivery-review/` 与 Android `build/reports/starter-delivery-review/`。管理端 211 项、Portal 94 项、工具 39 项通过，类型检查与两端 production build 通过。Android 串行 `test assembleDebug lintDebug assembleRelease :app:assembleDebugAndroidTest` 通过（7m 15s）；全部模块 2,840 项、12 项既有环境跳过、0 失败。

生成验证遇到 Windows 映射文件锁，失败日志保留；仅恢复该次 Ent 生成造成的残留，在隔离临时目录重生成 292 个 Ent 文件后逐个比较，无内容差异。其余生成链完成，374 文件检查中的两项 TypeScript 差异仅为生成器恢复 LF，重新生成后字节稳定；跨仓来源摘要、Android `--check` 和 gofmt 均通过。不提交 Ent 残留或无关生成变更。

设备范围与失败处理：App 整套 connected 报告为 252 项，238 通过、14 项环境/显式启用跳过。全模块命令继续执行 speech 时，两项真实录音成功上传测试返回 `NoSpeechDetectedException`，旧用例仍等待上传而超时；原始日志和音频统计已留档。测试调整为有声输入显式启用，并新增无需声学输入的录音取消清理场景；speech 18 项中 16 通过、2 项声学场景跳过，workspace 12 项中 11 通过、1 项文件系统环境跳过。没有降低生产信号阈值，也不把声学场景列为成功验收。

本轮真实 Core 首发及首次详情成功，Activity 重开后的详情加载再次超时。临时探针核对 access、runtime、投影与 UI emission；Mock 及真实 Core 探针运行通过，没有找到可确定的产品缺陷，探针已全部移除。Starter 端到端测试改用 Compose 1.12 的 v2 `createEmptyComposeRule`：其 StandardTestDispatcher 配合 UI 线程时钟推进，消除旧 Unconfined 调度器在后台线程续行的已观测差异。保留业务断言与原失败证据，不用重试包装通过，也不将该测试调度调整宣称为原 spinner 的确定性产品修复。

最终无探针构建通过三轮独立 Mock 重开场景；`core-android-final.log` 的 6 个浏览器用例和实际 Core Android 用例（24.374 秒）全部通过，实际请求与重开详情截图已复查。该轮为 generation 2、release `rel_83874daf-3ce9-493b-b3a8-7cd62e4e033b`、hash `sha256:abe7fbb5c196f9c91555a3d7c8f24683edadb61b105b5bd202fb3a1f335a9991`。Android 最终串行 `test assembleDebug lintDebug assembleRelease` 再次通过（41 秒），生产 Debug APK SHA-256 为 `182d9e62bd6d829ac47c04dfd0e7f095301333acdbceffe4d083215e97a02490`，测试 APK 为 `53ff017ec79787fe298bbef5a197fcfc1fdddcf1a26e78e8d1b30cd5c16a5439`。原失败不计入通过结果，真实供应商及持续有声输入场景仍未验收。

### 10.7 企业助手窄屏布局复核

用户反馈后，在正在运行的 device:real production SPA（本地 9100 端口）实际复现 390px 下常用入口编辑按钮超出卡片、整页横向滚动。根因是 ResourcesPage 的窄屏通用规则把 `.no-wrap` 行内的摘要 `.col` 设为 `flex: 1 0 100%`；此前只检查开场弹窗的几何边界，没有覆盖弹窗外的列表。修复将整行表单规则限定为可换行的 row，保留摘要收缩和编辑按钮宽度。助手 tabs 保留原有结构，窄屏减少内边距并启用外侧滚动箭头；长名称、技术标识允许换行，启用开关不被挤压。未改变 API、Draft owner、保存发布语义或 Android 数据。

实际网页检查覆盖基础名称/说明、助手启用、指令多行编辑、记忆新增/编辑/移动/移除、模型选择与 MCP 多选、入口新增/编辑/启用/移除、继承/独立 System 展示、背景新增/编辑/移动/移除、技术标识展开、关闭后重新编辑和快照预览。320px 使用临时长标题验证换行与按钮边界，390px 核对原企业助手两张入口卡片；临时编辑只留在浏览器草稿，检查后重新加载，已保存修订仍为 10、仍为原有 2 个助手。未发布测试内容或修改实际接入凭据。

`golden-path-authoring.spec.ts` 在既有真实 Hub/Relay/production SPA 流程中增加 320/390/768/1280px 五个详情分区及入口卡片的几何断言。新增断言先在原构建失败，修复后整套 `node scripts/e2e-harness.mjs` 通过，包含保存/刷新、继承恢复、错误定位、Preview/Review、发布和真实确定性调用链。组件测试 33 文件、211 项通过；typecheck、e2e:typecheck、production build 通过。此为浏览器布局/功能验证，不代表过期供应商密钥恢复可用。

本地证据为 `.artifacts/assistant-responsive-red.log`、`assistant-responsive-green.log`、`assistant-responsive-unit.log`、`assistant-responsive-typecheck.log`、`assistant-responsive-e2e-typecheck.log`、`assistant-responsive-build.log`；实操截图保存在 `.artifacts/assistant-responsive/`。失败日志保留，未放宽断言或加入自动重试。
