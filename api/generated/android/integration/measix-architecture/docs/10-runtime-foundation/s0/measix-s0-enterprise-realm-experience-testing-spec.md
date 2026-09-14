# S0.2 Enterprise Realm & Experience Foundation Testing 规格

> 状态：S0.2 System / Component Testing Authority
> 版本：2026-08-31
> 上位合同：`measix-s0-enterprise-realm-experience-contract-spec.md`
> 产品要求：`measix-s0-enterprise-portal-product-requirements.md`
> Wire 权威：`measix-s0-control-protocol.md`
> 下游测试：`measix-s0-enterprise-tool-gateway-testing-spec.md`（S0.3）、`measix-s0-android-client-testing-spec.md`（S0.4）
> 文档职责：定义 S0.2 Realm/A/B/C 体验基础、Session、Portal、Enterprise Update Feed 和 Android 最小纵向闭环的 required evidence；不规定 Gateway 工具行为、测试框架或 class/file。

## 1. 测试目标

S0.2 必须证明：

```text
Current S0.1 capability baseline remains reproducible
+ Snapshot v4 Realm/experience content is deterministic and client-safe
+ Android Personal/Enterprise Realm is isolated
+ Managed Assistant + seed + Starter works
+ Portal status and Enterprise Updates are usable
+ Hub has no transitional Enterprise Update MCP projection
```

## 2. Gate 分层

| Layer | Required evidence |
|---|---|
| T0 Contract | current OpenAPI/fixture/codegen/hash/schema strictness |
| T1 Domain | Definition/reference/date-window/session/realm rules |
| T2 Component | Hub/Admin/Feed/Portal/Android stores and projection |
| T3 Cross-component | Android↔Hub/Relay、Portal↔Hub；no Hub MCP projection |
| T4.2 S0.2 Product | real browser + real Android emulator/device + public topology |

## 3. Contract scenarios

- `ERX-C0-001` 当前唯一 Snapshot v4 fixtures 可复算；旧版本明确拒绝；
- `ERX-C0-002` Snapshot v4 完整包含 A/B/C、Policy、assistants/starters；
- `ERX-C0-003` assistant/model/MCP/starter refs are typed and complete；
- `ERX-C0-004` invalid/missing/disabled refs block publish；
- `ERX-C0-005` seed/starter normalization and deterministic hash/order；
- `ERX-C0-006` Snapshot contains no Enterprise Update body, Secret, Upstream or runtime route；
- `ERX-C0-007` Android generated artifact pins exact v4 OpenAPI/fixture manifest。
- `ERX-C0-008` seed may be empty; non-empty item validation and authored order are preserved; canonical Assistant/Starter arrays use stable IDs while equal display sortOrder uses starterId tie-break, with current-version deterministic evidence。

## 4. Managed Assistant / Memory / Starter

- `ERX-C-001` Admin creates and publishes one Managed Assistant with enabled Managed Model；
- `ERX-C-002` assistant model and optional Direct Managed MCP references are closed and do not invent an Enterprise Update MCP；
- `ERX-C-003` multiple non-empty memory seed items project read-only；
- `ERX-C-004` mutable Enterprise Local Memory remains separately writable；
- `ERX-C-005` memory tool cannot update/delete seed；
- `ERX-C-006` new generation replaces seed but preserves Enterprise Local Memory；
- `ERX-C-007` Starter renders title/description/order and pre-fills prompt；
- `ERX-C-008` Starter click never auto-sends or invokes tool；
- `ERX-C-009` Personal Realm exposes none of assistant/seed/starter references。

## 5. Enterprise Updates domain

- `ERX-UPD-001` Admin Draft→Publish makes item client-visible；
- `ERX-UPD-002` Withdraw removes item after successful refresh；
- `ERX-UPD-003` Feed update changes Feed revision/ETag but not managedGeneration；
- `ERX-UPD-004` Client sees only PUBLISHED items ordered newest-first；
- `ERX-UPD-005` cached list remains usable with explicit stale state on outage；
- `ERX-UPD-006` local read marker does not become enterprise receipt/User Sync。
- `ERX-UPD-007` concurrent Publish/Withdraw atomically advance Feed revision with visible content; a list response and ETag use one consistent view; Draft edits do not change public Feed revision。
- `ERX-UPD-008` conditional Feed reads distinguish normalized query/representation and deployment-date boundaries; timezone/DST transitions cannot return a stale 304 or exclude valid end-date items。

## 6. Enterprise Update query projection

- `ERX-B-001` no dates + omitted limit returns latest 10；
- `ERX-B-002` no dates + limit N returns latest N；
- `ERX-B-003` start only returns start-date through today；
- `ERX-B-004` end only returns latest items up to end date；
- `ERX-B-005` both dates use inclusive closed interval；
- `ERX-B-006` start > end returns typed invalid argument；
- `ERX-B-007` limit 0 or >20 returns typed invalid argument and is never silently clamped；
- `ERX-B-008` Deployment timezone and newest-first ordering are explicit；
- `ERX-B-009` overflow sets `truncated=true`；
- `ERX-B-010` Client/Portal Feed uses public client session authority and is unavailable in Personal Realm；
- `ERX-B-011` Hub exposes no MCP initialize/tools/list/tools/call projection; future Gateway private typed read endpoint is absent from Android/Portal public ingress and cannot mutate Update authority；

## 7. Realm / Enrollment / Session

- `ERX-REALM-001` app starts PERSONAL with no Managed content；
- `ERX-REALM-002` QR/paste Enrollment atomically establishes Binding/Credential/Session；
- `ERX-REALM-003` entering ENTERPRISE checks session and managed state；
- `ERX-REALM-004` Realm switch preserves runtime Conversation/Memory/Attachment/Workspace realm and subject; the same user Assistant definition runs with isolated data；
- `ERX-REALM-005` Enrollment initializes idle expiry; successful authenticated Refresh atomically rotates Refresh Credential and extends server idle expiry to seven days；
- `ERX-REALM-006` ordinary API/Runtime/Snapshot/Feed/Portal load and Personal foreground/background work do not independently renew enterprise Session；
- `ERX-REALM-007` expiry/revoke/Disconnect returns active realm to PERSONAL without deleting Personal Local；
- `ERX-REALM-008` no token/credential in Settings/backup/log/UI。

## 8. Trigger scenarios

- `ERX-TRG-001` first Enrollment fetches Snapshot v4 + first Enterprise Update page；
- `ERX-TRG-002` enter Enterprise Realm performs lightweight state check；
- `ERX-TRG-003` manual sync checks Snapshot and Feed independently；
- `ERX-TRG-004` each new enterprise interaction passes managed preflight；
- `ERX-TRG-005` foreground check is rate-limited and only active for Enterprise Realm；
- `ERX-TRG-006` Push/background hint cannot bypass pull or renew Session；
- `ERX-TRG-007` active interaction never silently switches captured realm/generation; its next stale Relay request receives 428 and terminates without replay。

## 9. Portal scenarios

- `ERX-PORTAL-001` Native Host works when Portal/network unavailable；
- `ERX-PORTAL-002` status shows identity/session/readiness/generation/last sync without secrets；
- `ERX-PORTAL-003` Sync/Refresh/Open/Disconnect buttons use authoritative commands；
- `ERX-PORTAL-004` WebView only loads approved Portal origin；
- `ERX-PORTAL-005` Android requests a server-issued short Web Session bound to the approved Portal origin/enterprise principal; Refresh Credential never reaches JavaScript；
- `ERX-PORTAL-006` list/detail/empty/offline/withdraw behavior matches Product Requirements；
- `ERX-PORTAL-007` bridge rejects unknown origin/method/invalid arguments；
- `ERX-PORTAL-008` Portal expiry/revoke has recoverable UX and no realm data leak。
- `ERX-PORTAL-009` grant expiry/replay/concurrent exchange/restart preserve one-use semantics; Portal Cookie cannot access Admin, Snapshot, Runtime or grant issuance; session close requires Origin + CSRF and does not renew/revoke the parent Session。

### 9.1 新版配置与手机能力必需证据

- `ERX-POL-001` v4 显式五项 Boolean，新建默认 false；缺字段/null 拒绝；旧原型版本拒绝，不存在旧策略采用或升级路径。
- `ERX-POL-002` 客户端拒绝非当前 schema，不把任何单项授权重解释为其他用户资源授权；个人域不被企业配置错误阻断。
- `ERX-POL-003` 五项开关允许/禁止/收紧/恢复直接作用于用户原定义，无副本、无全局删除/锁定；用户凭据用于用户资源且不会发送到企业平台。
- `ERX-POL-004` 用户助手与受管助手共存，受管定义不可改；本域使用参数/扩展偏好不复制个人助手、不覆盖企业固定字段，不以提示词/Header/Body 绕过路由认证或准入；引用逐项授权，禁止引用阻止执行并有原因；Direct Managed MCP 强制启用与助手绑定过滤同时成立。
- `ERX-INIT-001` 仅从当前完整配置初始化；非当前开发配置/数据库直接清理，不提供转换、双读或双写入口。
- `ERX-DATA-001` 新建运行数据必须带明确 Realm/主体；同用户助手跨域数据隔离，个人备份恢复不覆盖企业数据/认证/下发配置。
- `ERX-SIM-001` Debug/Release 正式企业入口与持续本地来源标识覆盖同一域/解析/命令/运行/持久化；内置企业示例使用确定性 I/O，用户主动选择的获准资源使用用户执行链。隔离测试传输证明凭据/端点正确且没有意外真实请求，失败不在示例/真实服务间回退，退出/重启不覆盖真实绑定。
- `ERX-PHONE-001` 当前受信顶层网页相机/麦克风调用经过权限与用户操作，取消/切域/页面替换/失效不会交付迟到结果或留下录制；结果不进入个人数据，无任意文件系统访问。
- `ERX-PHONE-002` 切回个人保留企业登录，断网不退出，主动退出本地完成且明确远端撤销结果；Portal 故障时原生状态和退出可用。真实平台设备证据与浏览器/本地示例分别报告。
- `ERX-POL-005` Search/Skill/注入/QuickMessage/本地工具/显式 Workspace 依独立准入规则可用，其底层受控资源不能旁路；用户子助手叠加 allowLocalAssistants 与原主从授权，继承父域/主体；显式失效引用提供本域重选且不修改共享定义。
- `ERX-JOIN-001` Admin 复制与扫码内容为同一完整 PLATFORM_ENROLLMENT 资料；原生扫码/粘贴共用解析器，拒绝超长、重复键、未知字段/版本/来源、错误类型/URL 和过期资料。code 不泄漏到 Discovery/日志/网页，目标 origin 明确，跨 origin 重定向拒绝。
- `ERX-JOIN-002` 本地一键/扫码/粘贴走同一验证/原子提交；本地资料不能用于真实 Enrollment；相同 Deployment/User 的本地与平台来源隔离，缺配置状态不假报 READY；完整配置文件不误当接入资料。
- `ERX-PORTAL-010` 同步按钮能力门禁、进行中防重复、成功版本/时间、无变化确认、失败保留旧状态、取消/超时后查状态均符合协议；原生页与网页同一命令，Feed 刷新不改变配置版本。
- `ERX-PORTAL-011` 本地/远端工作台使用相同 Bridge v3 原生操作、状态和媒体语义，来源与会话不混用；拒绝旧版本、错误文档和未知字段，bootstrap 缺失时不降级。录音由原生开始/停止 UI 完成，取消释放临时媒体，readMedia 分块与 EOF/总长度校验一致。
- `ERX-PORTAL-012` 本地读取 v2 的 context/Feed/详情只通过规定消息方法读取；实际 WebView 消息提供的 origin/顶层 frame、当前 documentId/主体/Session 全部校验，拒绝同源 iframe、错误/旧文档及远端宿主上的本地方法。静态资源拦截不承载旧本地 GET，不触发 DNS/Hub；无平台 Cookie/CSRF、无远端回退。remote/local 构建资源和契约 hash 可重放。
- `ERX-PORTAL-013` 默认最新 20、日期单边/双边/重置、企业时区边界、截断/空态和撤回在本地/远端一致；远端 HTTP 304 与本地 modified/notModified 分别验证。无条件读取必须返回完整内容，notModified 不得携带 feed，缺对应缓存/etag 不匹配按协议错误处理；退出/撤销后的相同 etag 请求不得命中缓存。更换查询取消旧读取，ETag/缓存不跨条件，迟到详情与列表不覆盖新查询。
- 原生接入资料的真实解析器须消费共享原文案例并记录输入摘要，验证 §8 的 UTC 接收子集和禁止截断精度规则；原生 Feed 与服务端须执行相同日期/时区案例，单独证明动态发布/撤回不推进配置 generation。各自手写相似测试不能替代共享案例消费。
- `ERX-PORTAL-014` 网页过期不假报母 Session 失效，来源拒绝不假报离线；原生状态失败可重试，关闭/退出防重复，宿主缺失有解释，手机布局/键盘可操作，所有已授权来源复用相同页面流程。
- `ERX-PORTAL-015` 原文档请求延迟期间，同 origin 导航、刷新、切域、退出、重登后创建新文档：旧 reply proxy/响应/documentId/requestId 不能作用于新页面，新页面不得接收旧结果或错误；取消、期限和媒体清理不依赖 JS 回调。JVM/浏览器替身之外，必须提供真实 WebView 设备证据，不能以伪造 fetch/304 通过证明 Android 能力。

- `ERX-PORTAL-016` 切域发布前确认旧宿主与该站点 Cookie、缓存、存储清理完成；包含非根路径 HttpOnly Cookie。失败不发布新空间，重试不提前复活旧文档；不删除无关站点数据。原生退出确认取消、到期、关闭与迟到确认分别验证，已接受退出由原 Session owner 收口，不等待 JS 成功回调。
- `ERX-PHONE-003` 采集取消、页面替换、到期和退出时先撤销交付权限，停止并等待原采集写入者，再清临时文件；旧写入结束不得产生新文档可见句柄。分块读取校验范围/进度/末尾，句柄只对原主体/文档有效。浏览器验证展示与取消消息，原生验证写入者/权限/文件，设备验证实际硬件，不互相替代。
- `ERX-CONTRACT-016` 共享 v4 完整资料在服务端与真实客户端使用同一输入摘要；仅接受当前版本，v4 缺策略/null/错误版本/断引用/秘密泄漏明确拒绝。HTTP 扩展字段与 Bridge 封闭字段分别验证；完整状态序列包括身份成功但无配置、刷新丢响应恢复、重启、授权先于缓存及 428 下游零调用。

## 10. Golden Scenario

```text
Admin publishes Managed Model
→ Managed Assistant + memory seed + Starters
→ Enterprise Updates
→ Android starts Personal
→ Enrollment
→ Enterprise Realm syncs Snapshot v4 + Feed
→ user selects “最近企业动态” Starter
→ prompt is prefilled and user sends
→ assistant runs with the published Managed Model
→ local enterprise memory can evolve without modifying seed
→ Client/Portal show the same latest Enterprise Update authority
→ switch Personal
→ all enterprise assistant/starter/update/portal entries disappear
```

## 11. S0.2 Exit Gate

S0.2 only passes when all required `ERX-*` scenarios are Green on exact architecture/core/android/portal contract inputs; no critical skip/quarantine; current Snapshot v4 artifacts are reproducible under Control Protocol §10.10.1; real Android/Portal public-topology Golden Scenario and required phone scenarios succeed; final S0.2 Freeze Manifest is generated.
