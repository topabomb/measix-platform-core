# Android 真实平台接入：合同准备与执行清单

> 2026-09-12 合同要求与已执行清单。本文件不取代 architecture 的协议权威，也不另建阶段状态真源；当前实现状态仍由 [s0-execution-progress.md](s0-execution-progress.md) 维护。
> 本计划已获授权执行。MEASIX 从未发布，当前协议和数据库结构是唯一版本；移除旧原型兼容、迁移、收养路径及其专属样例/测试。旧数据库或配置可删除后重新初始化，不转换旧数据。Android 仓库及其数据不在修改范围。

## 1. 目标、范围和交付标准

本阶段交付可供 Android 维护方实施真实接入的明确合同：固定版本、字段映射、真实请求/响应、共享正反例、版本与文件摘要、服务端回归结果及后续接线说明。完成标准是“上游合同准备就绪”，不是 Android 真实接入已实现，也不是 S0.2 Freeze。

允许修改的项目：measix-architecture、measix-platform-core（含 Admin）、measix-enterprise-portal。Android 仅作为消费端事实进行读取，不改其代码、资源、文档，也不运行其生成或设备测试。

本轮不建设生产环境、不新增真实平台 Android source、不实现 Gateway/Snapshot v5，不加入 User Sync、远程执行或额外业务功能。候选提交、推送与发布不由本计划自动执行；未取得固定提交前，材料只能标为工作树联调候选。

## 2. 本次核对基线

| 项目 | 读取的当前事实 |
| --- | --- |
| Android | HEAD `87ae6056f`；0.0.20 本地来源与消费者已实现。真实接入 roadmap 仍为后续工作；LocalEnterpriseSource 对 Platform 材料返回 platform_enrollment_not_supported |
| architecture | HEAD `50bcf595196e8684895f5ed0626291985844308d` 加工作树修订；当前 S0.2 目标 Snapshot v4，v5/Gateway 属后续阶段 |
| core | HEAD `95e8e815d3f9505d2eda08c15101f77f3e8f0380` 加工作树修订；CurrentSnapshotSchemaVersion=4；认证、刷新轮换、Snapshot、Feed、Portal 均已有实现和相关测试源码 |
| Portal | 尚无提交基线；Bridge v3/localReadVersion=2；近期 Android 使用相同构建身份，实际消费和最新设备记录需随最终候选重新关联 |

Android 参考入口：

- [真实接入 roadmap](../../../rikkahub_mcp/docs/dev/android-enterprise-production-integration-roadmap.md)
- [已完成体验补充](../../../rikkahub_mcp/docs/dev/android-enterprise-experience-completion.md)
- [本地域配置模型](../../../rikkahub_mcp/app/src/main/java/net/weero/measix/pilot/data/enterprise/EnterpriseConfiguration.kt)
- [平台资料尚未接通的 source](../../../rikkahub_mcp/app/src/main/java/net/weero/measix/pilot/data/enterprise/LocalEnterpriseSource.kt)

## 3. 当前实现与验证入口

已修正的差异、当前交付身份和未完成阶段门禁统一见 [当前状态](s0-execution-progress.md)，以下保留本次要求和完成清单，不重复保留失效的执行前代码描述。

## 4. architecture 执行清单

### A1 固定本次接入 profile

- [x] 明确 protocolVersion=1、Snapshot v4、Bridge v3、localReadVersion=2；接入资料 formatVersion=1 与这些版本独立。
- [x] 明确当前服务声明的版本、当前客户端必须实现的 profile、旧 v1/v2 不再支持；v5/Gateway 不进入 S0.2 首次接入前置。
- [x] 对齐响应扩展字段、未知 enum/版本与严格原生 Bridge 校验的差别，不为了修 schema 将所有响应统一改成未知字段即失败。
- 修改位置：Control Protocol、阶段索引；仅在确有语义缺口处修改权威段落。
- 验收：每个版本的当前/未实现状态明确，OpenAPI 和候选声明均可对应。

### A2 明确平台字段到客户端消费模型的边界

- [x] 列全身份与来源：可信 origin、deployment/user/device/session、sourceNamespace 的派生/绑定、同名不同来源、重登与同源地址规范化；不新增与既有平台 ID 竞争的身份。
- [x] 列全资源：Provider、Model、TTS、HTTP-ASR、Direct MCP、Assistant、Seed、Starter、policy 默认值与本域偏好。
- [x] 明确 Seed 顺序/空数组/配置更新与运行记忆分离；Android 适配所需内部标识不成为平台 wire ID，不依赖展示名合并。
- [x] 明确企业子助手关系不属于现行 Definition；多个角色默认槽位、私有 binding、本地场景参数不自动成为 v4 字段。已有用户子助手使用仍遵循五项准入。
- 修改位置：Experience Contract/生命周期与标识符文档的相应权威段落；具体 Kotlin 字段映射写在 core 接入说明中，不将实现类名塞进产品协议。
- 验收：每项本地模型字段都有“平台提供、派生、本域偏好、此次无对应能力”之一的处理，无隐式凭据或数据复制。

### A3 整理真实身份与配置的状态时序

- [x] 固定 Enrollment 一次性消费、Bootstrap、无 active Release/待配置、Managed State、Snapshot 获取/ETag/授权后缓存判断与 whole-state 应用顺序。
- [x] 固定 Refresh Idempotency-Key、旧 token/同 key 丢响应恢复、不同 key 冲突、恢复窗口和重启；普通请求不续期。
- [x] 固定 generation 更新、428 forwarded=false 后同步但不重放、网络未知结果、退出本地完成与远端撤销未确认的区别。
- 验收：输出正常、并发、丢响应、过期、撤销、重启的状态表；不改变已实现且符合合同的轮换算法。

### A4 补齐跨端验收边界

- [x] 将切域前站点清理完成、失败/重试、保留无关站点、原生确认取消/期限、采集停止后文件清理和旧文档拒绝细化到已有 ERX 案例。
- [x] 区分本地设备已验证、上游确定性系统测试、真实平台设备联调和 Freeze；关联已有测试，不把已完成的 Android 接线再次列为开发任务。
- 验收：每个案例标出上游可执行检查与 Android 后续验收责任。

## 5. core 执行清单

### C1 修正 Snapshot 导出的版本约束

- [x] 先增加失败案例：旧版本应拒绝；当前 v4 缺任一策略、null、错误类型、缺集合、错误版本必须按规定处理。
- [x] 用当前唯一 Snapshot/Policy 表达可执行约束，检查 Go/TS 生成结果；五项策略必须为布尔值。
- [x] 检查对应 HTTP 响应校验和 Android 导出，删除旧版本 canonical/hash 分支及策略升级代码。
- 范围：api/client、必要的 api/admin、生成配置、internal/contract、capability 相关回归。
- 验收：实际 compiler 输出通过当前版本校验，v4 反例拒绝；HTTP/生成消费者没有偷偷放宽要求。

### C2 提供完整 v4 共享资料组

- [x] 新建当前 Discovery、Enrollment、Bootstrap、Managed State、完整 v4 Snapshot 与 304/错误示例；所有主体、generation、resource 引用保持一致。
- [x] v4 覆盖模型/Provider、TTS、HTTP-ASR、Direct MCP、至少两名助手、Seed 有序/空数组、Starter 排序/禁用及五项策略允许/禁止组合，不包含 Gateway。
- [x] 增加断引用、禁用引用、秘密/内部地址泄漏、ETag 不一致、错误 deployment 等反例；明确哪些由 schema、领域校验、HTTP 或客户端状态测试验证。
- [x] 数据来自 compiler/真实 handler 或已有测试 fixture owner；不手工伪造 snapshotHash，删除旧 discovery/v1/v2 样例。
- 验收：资料组能独立解码与校验；服务端测试和交付导出引用同一份文件，当前 hash 可复算。

### C3 将既有身份/下发能力变成可交接证据

- [x] 先运行现有 identity/httpapi/capability 测试；已有并发刷新、恢复与退出测试复用，不重复建设服务。
- [x] 核对实际 HTTP status/header/body：Enrollment 201、deviceName、Refresh 必填 Idempotency-Key、sessionIdleExpiresAt、Bootstrap/Discovery live version、Snapshot ETag/404/304、撤销优先于缓存。
- [x] 对未覆盖的 HTTP/重启/丢响应边界补真实测试与共享时序输入；仅修确证偏差。
- 验收：端点说明来自实际 handler 与测试结果，不把服务函数测试当作完整 HTTP 互操作。

### C4 固定 Runtime 首次联调请求

- [x] 列出公开 origin + runtimeApiBase + resource ID + runtimePath 的组装示例；确认没有重复路径、跨 origin 或内部地址。
- [x] 固定 Model/TTS/HTTP-ASR/Direct MCP 的 clientProtocol、header、token、model key、内容类型、流式/二进制/错误读取与取消方式。
- [x] 复核 428 的 code/targetManagedGeneration/forwarded/requestId；验证拒绝时下游调用为零，未知结果不自动重试。
- [x] 将已存在 required-profile/Relay 测试作为基础，补与新 v4 资料组对应的公开请求检查；不以 deterministic Adapter 通过宣布真实厂商资格通过。
- 验收：Android 可据示例实现请求，无需猜测本地 binding 如何替换；本轮无需运行真实 Android。

### C5 扩展 Android 可消费的导出包

- [x] 复用现有 generate-android-wire 与资料 owner，包含 Client OpenAPI、当前版本校验输入、v4 资料、身份/状态时序、428、enrollment、Feed、Portal 及全部 schema 引用依赖。
- [x] 输出 schema/profile 版本与生成器信息；包内容由生成脚本从源文件直接复制，不维护摘要清单。
- [x] 在独立临时目录验证包不依赖 sibling checkout，重复生成稳定；篡改、缺文件、未列出文件与错误版本可检测。
- 验收：Android agent 取得一个包即可定位所有输入及预期结果。该包不是 Portal 静态包，也不产生第二套协议真源。

### C6 更新当前状态并形成接线说明

- [x] 更新 s0-execution-progress.md 和相关过时交接段落：Android 0.0.20 本地完成、真实 source 未实现、新 Portal 身份，移除不再适用的旧审查报告。
- [x] 在 docs/api-contracts.md 等既有入口挂接字段映射、接口序列、导出说明和测试结果，避免再建立平行 living status。
- [x] 准备最终基线清单：architecture/core/Portal 源码状态、生成与包摘要、Android 只读参考 commit、验证范围和剩余外部门禁。
- 验收：不存在“等待 accessor 首次复测”等错误当前待办；未提交工作树不能标成已冻结/已发布 commit。执行中若未授权提交/发布，明确交付为待固定提交的候选。

## 6. Portal 执行清单

### P1 核验最新页面与共享合同来源

- [x] 按 core 修正后的 Client/native 导出顺序生成，确认同一个 Bridge v3 owner、local-read v2 与远端 Cookie 读取保持边界。
- [x] 核验最新页面源码、remote/local build identity、实际 Android 已消费包和文档记录；更新旧包指引，不回退已经完成的 UI。
- [x] Starter 继续由原生宿主提供，不新增打开聊天 Bridge、网页配置写入或 Token 入口。
- 验收：类型/校验器无手工 DTO 分叉，两个构建与同一次源码/合同输入相符。

### P2 复验远端与原生动作交互

- [x] 在既有真实 Hub E2E 上验证 grant/exchange、受限 Cookie、Feed 日期/详情、会话到期/撤销、CSRF 关闭以及母 Session 不被网页关闭误撤销。
- [x] 检查并补缺失的 logout 取消/超时后恢复、媒体多块读取/途中取消/失效、外链迟到反馈与旧文档隔离；优先扩展现有组件/浏览器测试，不重复已有通过覆盖。
- [x] 保留 accessor 销毁/重建测试；原生 fixtures 仅证明网页边界，不计作设备能力证据。
- 验收：最新 UI 在 remote/local 均通过对应测试，已通过的本地硬件测试不替代远端 WebView 接线。

### P3 交付候选构建与消费说明

- [x] typecheck、单元、交付校验、format、双 production build、适用的真实 Hub E2E 按顺序执行。
- [x] 校验资源与合同摘要，按当前代码/文档重新生成需要的交付包，记录相对旧包的差异；没有页面变化也必须如实区分“重验”与“修复”。
- 验收：最终报告与实际交付包身份一致，说明 Android 后续远端宿主接线和设备验收范围，不修改 Android 固定资源。

## 7. 执行顺序和停止条件

1. A1–A3 明确基线和字段/状态语义；A4 明确验收责任。
2. C1–C2 收口 schema 与完整 v4 输入；C3–C4 验证真实服务行为，发现问题后按 Red → Green 修复。
3. C5 完成可重现导出；然后执行 Portal P1–P3。生成必须在测试/构建前完成，同一 checkout 不并发生成和验证。
4. C6 汇总三仓库候选与证据，交付 Android 接线文字。

如发现需要新增现行 S0.2 明确排除的产品字段或新协议版本，先把冲突与影响记录在权威文档的待裁决项，不能为了迁就本地模型悄悄扩展 wire。常规映射、明确规则的补写、回归与实现修复不要求 Android 先修改代码。

完成后应能回答：Android 连接哪个版本、发送什么、收到什么、如何验证/应用、失败如何处理、按哪份案例测试。真实网络环境部署、Android 生产 source 和端到端设备验收属于下一阶段；它们仍需完成，不能据本计划完成宣布 S0.2 通过。
