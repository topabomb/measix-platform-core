# 协议响应扩展与消费者稳定性

语义由架构 Control Protocol §2.1、§10.10.3 和 §11 拥有；本文件记录 Core/Portal 的落点与交付边界。Core 支持 API v1、Snapshot v4/v5 和 Bridge v3；Android 源码与设备验证由其仓库拥有。

## Core 实现

响应扩展、已知字段和命令的接收规则直接遵循 Control Protocol §10.10.3，不在本仓库维护第二份兼容规则表。

Admin 写入与 Client 下载的 ManagedPolicy 已知字段必须一致，但对象扩展规则按方向不同，合同测试不再要求二者 `additionalProperties` 完全相同。Client schema 省略 `additionalProperties: false`，采用 JSON Schema 默认可扩展语义；不添加生成 DTO 的业务字段副本或保留未知值的执行入口。Go 的类型化投影继续只解释已知字段。Admin 与前端同构建交付；它的共享写入 schema 保持封闭，响应读取不能借此建立未知字段拒绝规则。


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

## 验证入口

在 `backend` 执行 `go test ./internal/contract ./internal/hub/capability ./internal/hub/httpapi -count=1`；重点为 response_extensions_test、v4 bytes/hash、已知错型/null、命令封闭和生产非泄漏。重新生成消费者材料后比对权威源；Portal 在其仓库运行 STANDARD/CUSTOM 实际 Hub 浏览器路径，Android 按支持的真实构建独立验证。历史测试计数和候选提交不作为当前 Green，证据边界见 [testing](testing.md) 与 [release](release.md)。
