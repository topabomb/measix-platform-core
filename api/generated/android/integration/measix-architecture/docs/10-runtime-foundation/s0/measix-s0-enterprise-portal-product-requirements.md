# S0.2 Enterprise Portal MVP 产品要求

> 状态：S0.2 Product / UX Authority
> 版本：2026-08-31
> 上位合同：`measix-s0-enterprise-realm-experience-contract-spec.md`
> Wire 权威：`measix-s0-control-protocol.md`
> 测试权威：`measix-s0-enterprise-realm-experience-testing-spec.md`
> 文档职责：定义 Android Enterprise Native Host、Portal WebView、企业状态、操作按钮和企业动态的首版用户任务；不定义 Compose/Vue 组件树、路由库或 CSS。

## 1. 产品定位

Enterprise Portal 是企业用户工作台，不是 Admin Console。S0.2 交付可用宿主、企业状态、企业动态及受限拍照/录音与预览，为后续同步、备份、经验贡献、通知和任务保留入口边界。

## 2. 信息架构

```text
Android Enterprise Realm
├─ Enterprise Native Host
│  ├─ Enterprise Status
│  ├─ Sync configuration
│  ├─ Refresh enterprise updates
│  ├─ Open workbench
│  └─ Disconnect
└─ Enterprise Portal WebView
   ├─ Status summary
   ├─ Enterprise Updates
   └─ Enterprise Update detail
```

Native Host 在 Portal/网络不可用时仍必须可进入，并能表达本地 Binding、Session、Applied generation、最近同步和恢复动作。

## 3. 企业接入

用户从 Personal Realm 使用扫码或粘贴 Enrollment 资料“接入企业”。S0.2 不使用“完整账号登录”表述。

原生空间入口和设置中的企业接入在 Debug/Release 均可见；Portal/网络不可用时原生状态和退出入口仍可访问，本地示例可离线接入，真实平台接入必须完成服务端验证。本地版本另提供“体验示例企业”；一键、扫码与粘贴共用 Control Protocol §8 的版本化资料解析和验证流程。扫码在企业登录前可用，不属于 Portal 的拍照按钮；完整企业配置文件由原生“导入本地企业配置”处理，不能粘贴进网页当作登录资料。

真实平台接入成功后：

1. 显示企业名称、用户/设备摘要；
2. 建立服务端权威七天 rolling idle Session；
3. 首次同步 Snapshot v4 和 Enterprise Update Feed；
4. 进入 Enterprise Realm；
5. Android 向平台申请短期 Web Session 后打开 Portal；签发权威在服务端，不在 Android。

失败、过期或已消费的 Enrollment 不留下 half binding。

本地示例使用独立来源与本地验证、配置及网页会话，不调用真实 Enrollment/grant；持续标明“本地模拟”。身份已验证但配置未就绪时显示明确状态并保留同步/退出入口，不能伪装成完整可用。示例能力与真实平台验收分别报告。

## 4. Enterprise Status

至少显示：

```text
enterprise display name
connection/session state
session idle expiry or re-auth required
managed readiness
applied managed generation
last configuration sync
last enterprise-update refresh
recoverable error summary
```

状态页不能显示 access/refresh token、内部 URL、Upstream、Secret 或完整诊断 payload。

工作台持续显示“本地示例”或“企业平台”。来源由受信宿主和构建确定，网页不提供来源切换。企业登录有效、网页会话有效、配置已就绪是独立事实；状态未知显示“—”。仅从网页 401/到期不能推断企业母 Session 已失效，应返回原生企业页核验后重新打开。

首次网络失败提供“重新连接”；已验证会话内动态失败保留该查询缓存并提示非最新；无缓存显示可重试错误。配置失败保留最后确认版本，超时显示结果未确认并允许查询。网页失效立即清理企业内容和媒体，提供“返回企业页面”，由原生判断重新打开或重新接入。权限/来源拒绝不能伪装网络问题。关闭与退出在执行中防重复；网页宿主不可用时说明需要从 Android 打开，不提供无效的原生操作。

## 5. 操作按钮

### Sync configuration

执行 Managed State preflight；只有 generation 变化时下载、验证并原子提交 Snapshot。同步不会重放当前失败 interaction。

原生状态页和 Portal 按钮调用同一配置同步命令；本地来源使用同一验证/发布边界。Portal 等待 Bridge v3 `refresh` 完成再显示已确认版本与同步时间，进行中防止重复点击；不支持时禁用并说明，失败保留最后确认状态和可读错误。同步不是 User Sync、聊天备份或文件上传，精确并发/取消语义见 Control Protocol §8。

### Refresh enterprise updates

使用 Feed ETag/revision 获取最新内容，不改变 `managedGeneration`。

### Open enterprise workbench

真实平台 Portal 是企业提供的远端 HTML 站点，由 Android WebView 承载。Android 先确认企业 Session 可用，再向平台换取短期、受限、单 Portal origin 的 Web Session。生产远端站点不可因网络失败静默改成本地页面。平台接入前的本地示例允许随包 HTML 和独立本地会话，使用同一宿主、Bridge v3 和工作台产品行为，展示不同来源且不伪造生产授权。WebView JavaScript 不读取 Android Refresh Credential；本地/远端资源加载的实施衔接见 Implementation Decision。

### Disconnect

必须二次确认。主动退出与切回个人域分别提供明确入口。退出清理本地 Binding/Credential/Managed state/Portal Web Session 并回到 PERSONAL，同时尝试撤销服务端母 Session；失败时明确显示远端撤销未确认，不能谎报撤销成功或因离线阻止本地退出。个人配置和个人运行数据不删除。企业运行数据保留/导出仍需独立策略，不能将清理认证状态解释为已获准导出或删除全部业务数据。

## 6. Enterprise Updates

用户界面正式名称为“企业动态”。列表按 `publishedAt` 从新到旧显示：

```text
title
publishedAt
category badge
severity badge
short content preview (rendered by contentFormat)
local unread/read marker
```

详情显示 title、publishedAt、category、severity、content（按 contentFormat 渲染）。S0.2 的 read marker 只需设备本地保存，不进入 User Sync，也不构成企业回执。

工作台默认明确请求最新 20 条；提供开始日期、结束日期、“查询”和“重置为最新”，两日期允许各自为空。日期按企业时区解释，起止包含当日；单边条件遵循 Control Protocol：仅开始日期时截至企业今天，仅结束日期时截至指定日。开始晚于显式或默认结束日期时就地提示且不发请求。一次结果最多 20 条，truncated 时说明只展示该条件下最新 20 条，不能表示已读完全部历史。重置清空日期并重新查询。没有结果显示该条件无动态。

缓存与 ETag 按来源、Deployment/User/母 Session 和规范化查询条件隔离。更换条件关闭详情、取消旧查询；旧响应不能覆盖新条件。读取失败只能展示同条件缓存，不能保留上一查询并换标题。已读标记仍属于母 Session，与筛选条件无关；本期不增加分页、全文搜索、分类筛选或回执。列表/详情返回网页前均使用同一发布权威，刷新与查询不改变配置 generation。

空态、离线缓存、刷新失败和已撤回内容必须有确定行为：

- 首次无内容：显示正常空态；
- 有缓存且网络失败：显示缓存和“可能不是最新”状态；
- 无缓存且网络失败：显示可重试错误；
- 已撤回内容在下一次成功刷新后从可见 Feed 移除；
- Feed 更新不触发 Managed Snapshot 下载。

## 7. Managed Assistant 与常用入口

Enterprise Realm 展示受管助手的名称、说明、企业来源和只读状态。助手页展示企业维护的“常用入口”，按 `(sortOrder, starterId)` 排序；展示排序不改变 Snapshot canonical order。

点击入口只预填提示词：

```text
select managed assistant
→ open/create Draft Conversation
→ prefill prompt
→ user edits or sends
```

不得 auto-send 或在点击时直接执行 MCP。

## 8. Session UX

- 切回个人域保留企业登录及其原有到期时间，但销毁 Portal 文档、取消手机能力调用并清理 WebView 会话；切回企业时重新检查身份与配置，不自动续期；
- 必须有原生接入/重新登录与退出入口；扫码和粘贴使用相同接入资料校验，资料包含平台地址与接入认证材料，验证成功后才建立绑定；
- 网络失败保留登录，显示离线与重试；不因超时或断网执行退出。Portal 不可用时原生状态页与退出入口仍可用；
- 进入 Enterprise Realm、打开 Portal、主动同步或发起企业交互时，Android 只在 Access Token 需要更新时调用 Refresh；
- 只有成功的 authenticated Refresh 按服务端时钟续期七天 idle Session，并原子轮换 Refresh Credential；普通 API、Runtime、同步和 Portal load 不单独续期；
- Personal Realm 启动、后台轮询或 Push 到达本身不续期；
- Session 过期后 Personal Realm 继续完整可用，Enterprise Realm 显示重新验证/接入动作；
- Portal 短期 Web Session 过期只关闭/刷新 Portal，不暴露或复制 Android credential。

## 9. Native Bridge

基础状态能力保留：

```text
get host/app/session status summary
request close
request native refresh
open approved external link through Android
```

相机拍摄与麦克风录制是明确交付需求：新版 S0.2 必须完成受信网页发起、原生权限/取消、结果仅回到请求文档的闭环，并进入本地示例验证及真实设备 Exit。企业接入扫码由原生接入入口交付，不要求先登录 Portal 才能扫码。

网页/原生消息与媒体传输以 Control Protocol §8 Bridge v3 为唯一权威；组件实现与设备验收后才形成冻结证据。原生只向当前已验证顶层 origin 的企业页面授予明确能力，按系统权限和用户操作决定，禁止后台静默录制；离开页面、切域、Session 失效或退出立即取消，迟到结果不得交给新文档或新主体。媒体结果归属请求时企业域/主体，不自动写入个人聊天或上传服务端。网页无配置/数据库/任意文件系统访问权，不得取得 Refresh Credential。任意文件选择、媒体上传业务和完整企业数据保留策略仍需独立决策，不从相机/麦克风能力推导。

## 10. 非目标

- 企业动态搜索、评论、回执、点赞；
- Push、通知中心、任务中心；
- 同步/备份/上传的实际业务流程；
- Skill/Agent Space/Remote Agent 页面；
- 多窗口、多 Portal origin 或任意网页导航；
- WebView 直接修改 Android Settings、Assistant、Memory 或 Managed Store。

## 11. Product Exit

真实 Android + 真实 Portal production build 至少证明：扫码/粘贴接入、状态展示、配置同步、企业动态刷新/列表/详情、Managed Assistant 常用入口、Portal Session expiry/recovery、相机/麦克风权限与取消及结果隔离、切回个人域保留登录、主动退出和 Personal Realm 隔离。浏览器替代 native grant 或本地示例 I/O 不证明真实平台设备 Exit。
