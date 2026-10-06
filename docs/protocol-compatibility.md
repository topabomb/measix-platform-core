# 协议响应扩展与消费者稳定性

语义由架构 Control Protocol §2.1、§10.10.3 和 §11 拥有；本文件记录 Core/Portal 的落点与交付边界。本轮不修改 Android 源码，不改变 API v1、Snapshot 支持集合 v4/v5、Bridge v3 或已发布数据。

## 问题与处理

Android 截图只提供 `unknown_platform_field`，没有字段路径，无法据此确认具体新增字段。审查发现旧架构和 Client OpenAPI 明确封闭 Snapshot 子对象，Portal 额度与 Bridge 也拒绝未知字段。因此普通新增说明字段就可能使整份配置或查询不可用。仅修一个 DTO 或删除 Core 的某个字段无法建立稳定演进规则。

当前合同改为按通信方向处理：受支持版本内的响应对象递归忽略未知字段；写入命令继续拒绝未声明参数。忽略字段不等于理解新功能，也不改变权限与默认值。已知字段、必填、类型、null、范围、互斥、ID/引用、来源、身份、generation 和准入规则保留校验。

| 边界 | 当前处理 | 实现/验证 owner |
|---|---|---|
| Discovery、Enrollment/Refresh 响应、Bootstrap/ManagedState、Snapshot v4/v5 | 递归允许响应扩展；未知合法版本先分类不兼容；已知权限与执行枚举不猜测 | Client OpenAPI、共享 fixture、Go 合同测试；实际 APK 消费由 Android 验证 |
| Client/Portal 查询：Session、动态、工作区、用量、额度 | 新增字段不使已有投影失败；已知值仍验证 | Client OpenAPI；Portal 的生成 Feed 校验器和 usage 校验 |
| Bridge v3 响应 | 信封可扩展；result/error 互斥；按捕获方法校验结果，绑定原文档/请求 | Core Portal schema；Portal Bridge owner 与测试 |
| Bridge 能力声明与错误码 | 未知方法声明可忽略，不增加本端可调用方法；未知 code 保留为操作失败，中性提示 | HostStatus/BridgeError schema；Portal Bridge 测试 |
| Admin/Client 命令、Bridge 请求/params、Enrollment 资料、内部控制指令 | 未声明字段拒绝，避免意图被静默丢弃 | 各命令 schema 与现有 HTTP/来源/鉴权测试 |
| 未知权限模式、额度模式、内容格式或执行协议枚举 | 保持拒绝；新增含义先审查版本或定义明确兜底 | 不默认全部工具、不显示不限额、不按 HTML 执行未知格式 |

Admin 写入与 Client 下载的 ManagedPolicy 已知字段必须一致，但对象扩展规则按方向不同，合同测试不再要求二者 `additionalProperties` 完全相同。Client schema 省略 `additionalProperties: false`，采用 JSON Schema 默认可扩展语义；不添加生成 DTO 的业务字段副本或保留未知值的执行入口。Go 的类型化投影继续只解释已知字段。Admin 与前端同构建交付；它的共享写入 schema 保持封闭，响应读取不能借此建立未知字段拒绝规则。

Client 合同移除没有任何 Client operation 引用的 ManagedDraftContent、RuntimeBindingDefinition、TimeoutPolicy、ValidationIssue 旧 authoring/internal schema；这些对象仍由 Admin/内部合同拥有，不是受支持的历史 Client 响应。没有移除 v4 decoder、历史发布恢复或当前权限校验。

Core 的 Snapshot 读取先选择生成的 v4/v5 DTO，再进入历史 adapter，避免用 v5 字段类型检查 v4 扩展。PublishedContent 的下载开场校验与 Admin 草稿校验分开：前者容忍扩展，后者保持封闭；两者共享已知必填/null/格式/顺序规则。

Snapshot v5 的旧 `mcpServerIds` 和 Starter `description` 不再作为执行语义；若与完整有效的 v5 字段同时出现，视为无效用的扩展而忽略。它们不能替代必填的 `mcpBindings` 或 `openingSnapshot`。缺少现行必填仍拒绝，不能自动转换或扩大工具范围。

## 完整性与数据保全

现有 v4/v5 descriptor、稳定排序、ETag/body snapshotHash 和历史 bytes/hash 不变。本轮未知扩展不改变现有已知字段 descriptor。后续新增输出字段允许新发布生成新的 bytes/hash，须同步编译/验证工具并验证旧消费者；不修改历史发布，不为普通说明字段强制升版。影响执行或 required 的新含义仍须审查版本。接收端不自行对原始 JSON 另算 hash。合同测试向支持版本的所有对象层加入说明字段，证明可接收且 Core descriptor 未变，并保留已知错型/命令未知字段反例。

MCP Tool contractHash 是独立的完整 Tool 原始 JSON JCS 合同，仍包含 `_meta`、annotations 和扩展字段。不能将配置 DTO 的忽略策略用于 Tool 摘要。

客户端宽容不是服务端泄漏防线。Secret、Upstream 地址、内部路由和 Admin 模板元数据必须由生产投影排除；实际 Hub 编译与预算投影测试独立验证。原有 secret-leak 反例属于生产输出 guard，不能再叙述为下载消费者必须封闭。

不修改 SQL 历史，不清理 durable Draft 或原 Applied，不重写历史发布。旧 consumer 的严格 decoder 不会因 Core 升级自动改变；部署前按支持的实际构建验证，并保留原控制路径。

## Android 后续对接要求

Core-owned OpenAPI、Bridge schema、共享向量和集成导出已同步，本轮不改 Android 仓库。后续需在响应解析 owner 设置递归未知字段容忍，同时检查 mapper/领域层没有第二套未知字段拒绝；请求校验继续独立。仅改变一个全局 Json 开关不足以证明完成。

先识别合法版本，再做受支持版本的完整候选验证与原子提交；未知版本、坏内容、网络与身份错误分别处理。失败保全 Binding、历史和原 Applied，空间导航、个人域和退出不依赖配置成功；依赖配置的企业执行仍受当前准入限制。Portal 查询失败只影响当前视图。未知错误 code 不能推断身份撤销。

技术诊断提供稳定 code、非敏感 JSON 路径及原因，避免截图中的无路径错误；不记录原值、Secret、响应正文或内部地址。新增普通字段不形成错误。

验收包含真实支持 APK 与当前 APK：顶层/嵌套扩展、已知错型/null/必填、权限枚举、引用、未知版本、重复键/超限、失败后保全/恢复、Bridge 原文档关联。当前 Core/Portal 检查不能替代此项。

## 发布与验证流程

1. 在 architecture 审查字段含义：说明性新增无需版本升级；影响执行、授权、默认值或无兜底枚举时先明确版本。
2. 更新 OpenAPI → 共享 fixture → 生成材料 → 服务端/Portal；先观察最小失败测试再实现。
3. 验证历史 v4 fixture bytes/hash、当前版本已知约束、生产非泄漏、生成一致性、Core 全量和 Portal 的真实 Hub STANDARD/CUSTOM 路径。
4. 记录当前源码、合同、构建及测试证据；Android 未验收时明确标记，不能宣称完整端到端兼容或新的 S0 Freeze。

## 2026-10-06 验证记录

Red 已观察：普通新增字段使 Snapshot v4/v5、Discovery、Bootstrap、Refresh、Portal 额度/计量、Feed 和 Bridge 失败；Core 历史 adapter 对 v4 中的 v5 字段提前类型校验，PublishedContent 将下载扩展套用命令校验；Client 合同仍导出四项无 operation 引用的旧 authoring/internal schema。

Green 覆盖响应扩展与已知错误的双向用例、命令未知参数拒绝、v4/v5 版本投影、原开场字面值与历史 hash、真实 Hub 的私有模板投影隔离、完整 MCP Tool JCS 摘要。共享材料增加基础控制与 v4/v5 嵌套扩展接收样例；生产非泄漏反例从消费者拒绝规则移回生产输出 guard。

执行了 Core 全量 Go 测试及 vet、合同检查、Admin 265 项测试/typecheck/build、62 项工具脚本测试与 1 项 public-origin 测试；真实 Admin 浏览器 8 项通过，0 skip/flaky/unexpected，包含编制/发布、MCP、用量与桌面/320px 页面操作。Portal 101 项测试、typecheck/build 与 STANDARD/CUSTOM 各 2 项真实 Hub 浏览器测试通过；原生 Bridge 使用浏览器替身，未运行 Android 设备。受影响生成链另行重跑并比较，历史 v4 fixture 无差异。

具体源码/合同/构建摘要与提交身份记录在本地 `.artifacts/protocol-compatibility-verification.json`。这不是阶段 Freeze 或 Android consumer 验收。旧的 Preview/Starter 设备记录保留其原构建边界，不能据此宣称本轮新消费者已可用。
