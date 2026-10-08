# Testing、CI 与 TDD

本文维护 Core 可执行验证入口。必验行为和场景由同级架构仓库的[阶段阅读清单](../../measix-architecture/docs/measix-stage-document-index.md)及各 Component/System Testing Spec 拥有。固定版本和历史结果统一见[证据索引](s0-execution-progress.md)，发布晋级见[发布流程](release.md)。

## 验证层与执行边界

| 层 | 实际用途 |
| --- | --- |
| T0 | OpenAPI/fixture/生成材料合同、format、tooling、类型与 build |
| T1 | Go domain / Vitest unit、store 和 helper |
| T2 | 真实 SQLite、HTTP、streaming、静态托管及组件集成 |
| T3 | 真实 Hub ↔ Relay、deterministic Adapter/Test Client；smoke build tag |
| T4.1 | S0.1 生产 Admin 浏览器 → Hub/Relay → Adapter/公开 Test Client，CAP gate |
| T4.2 | S0.2 Realm/Portal/Android，按 ERX 规定验证 |
| T4.3 / T4.4 | S0.3 Gateway 三 daemon / S0.4 完整 Android profile；当前仍有未实现项 |
| Final S0 | 固定 S0.1–S0.4 组合与最终 SYS gate |

普通 CI 为确定性的 T0–T3 和必要构建，排除浏览器 T4 与真实外部资格。Go/Console/tooling 通过不能证明安装 APK、供应商调用、Freeze 或最终 Exit。

## 命令与测试位置

Go 普通测试在 `backend/**/*_test.go`；系统环境为 `backend/test/system/{harness,adapter,client,scenarios}`。浏览器进程和证据 owner 为 `scripts/lib/harness.mjs`、`scripts/e2e-harness.mjs`；`console/e2e/` 只维护浏览器动作/断言，不另造 daemon 编排。

从 `backend/` 执行：

```text
go test ./... -count=1
go vet ./...
go test ./internal/contract -count=1
go test -tags=smoke ./test/system/scenarios/ -count=1 -timeout 5m
go test ./test/system/adapter/ ./test/system/client/ -count=1 -timeout 2m
```

从 Core 根目录执行：

```text
npm run test:tooling
pnpm -C console typecheck
pnpm -C console test --run
pnpm -C console build
```

合同、Ent 或 fixtures 变化还须 `node scripts/checks.mjs generate`、`node scripts/checks.mjs drift`。普通 `go test ./...` 不含 smoke/candidate tagged 场景；`make ci` 只执行测试，不负责生成或 drift。定向 suite 通过后按受影响真实边界扩大验证，不重复无新增意义的全量检查。

数据库测试必须执行真实 ordered migrations，覆盖空库、历史 fixture 升级和数据保全、重复运行、逐文件 atomic failure 及备份恢复；清库不能证明升级正确。

版本关联定向检查为 `go test ./cmd/generate-android-wire ./internal/contract -count=1`（backend）与 `node --test scripts/release-versioning.test.mjs scripts/preview-contract.test.mjs`（Core 根目录）。它们验证导出身份、默认打包不依赖 Android 报告、拒绝覆盖，以及可选证据工具的失败分支；fixture APK/报告只测试工具逻辑。真实组合按[发布流程](release.md#android-端调整方案)复用现有用例并记录版本、产物摘要及结果链接；只有主动采用自动证据工具才需它的四类报告格式，原阶段必验场景不变。

## 真实边界与故障证据

测试默认隔离 DB/ports、合成凭据、无公共网络依赖，限制异步等待并可靠清理。SQLite、Hub/Relay 进程、HTTP/TCP streaming、生产静态 build 是被测边界时不得 mock 掉。deterministic Adapter 用于外部边界与故障注入，不替代平台组件或真实供应商资格。

Node 端口分配统一走 harness，排除 Fetch-blocked ports；探测有界并关闭 probe，不改主机网络或浏览器安全配置。初始化可以建立身份/密钥，被测业务对象通过声明的 Admin/公开产品 API 建立，不直接写库或走 Relay internal 捷径。

关键场景在测试名、metadata 或附近注释中关联权威 HUB/RLY/ADM/CAP/ERX/ETG/AND/SYS ID；普通 unit 不制造 ID。覆盖率只是诊断，没有百分比能替代必验场景。产品断言失败原样保存，不以 retry、降低断言或隐藏 skip 收口。明确 runner/基础设施失败可重跑一次完整 CI job，原因和失败记录保留。

## 浏览器与 Android 专项

`make s01-browser-candidate` 构建生产 SPA 后执行唯一 `node scripts/e2e-harness.mjs`。同一隔离 DB/Hub/Relay/Adapter 中依次进行 authoring/publish、MCP、五类 Runtime、Usage/System、topology、Admin 路由 review 和账号/无引用上游删除；不能拼接不同环境的通过结果证明业务闭环。`console/e2e/admin-accounts.spec.ts` 覆盖创建管理员、无密码晋升、他人密码重设、撤销后再次授予不会复活旧 Cookie、真实并发角色冲突刷新后的提示、保留发布引用保护，以及桌面/390px 的操作对话框。

Harness 保存所服务 SPA 的固定副本与 build hash，避免共享 dist 重建导致混合 chunks。`e2e-admin-build.json` 绑定本次页面构建；默认 Playwright JSON 为 `.artifacts/e2e-playwright.json`，失败留 trace。手工检查用 `node scripts/e2e-harness.mjs --keep --manual`，登录资料只写隔离私有目录；按提示继续后才执行后续自动阶段。

### Starter 与协议兼容

Go contract/capability/httpapi 验证 v4/v5 分流、历史 bytes/hash、递归响应扩展、已知错型/null/required、命令封闭和生产非泄漏；Console 验证 Draft/CAS、未保存编辑、开场继承、定位及冲突。当前规则见 [API 参考](api-contracts.md)。

显式设置 `MEASIX_E2E_ANDROID_SERIAL` 可在浏览器发布后开启 native lane。例如 PowerShell：

```powershell
$env:MEASIX_E2E_ANDROID_SERIAL = '<dedicated-emulator-serial>'
node scripts/e2e-harness.mjs
```

先在专用 fresh-unbound emulator 安装匹配 Debug/androidTest APK；harness 不安装、不清数据，拒绝保留的生产 demo emulator。可用 `ADB` 指定工具。Admin API 签发临时接入资料，adb reverse 保持 canonical origin，Android 实际执行 HTTP sync、UI/send、Room 读回及详情重开；Adapter 校验发布开场与未重复 Assistant System。合成请求捕获、临时资料和 reverse 在 finally 清理。该 lane 不等于正式签名/R8、OEM 真机、实际录音或真实模型 gate。

原生兼容验证固定受支持旧构建和当前构建，包含 current/retained/future format、顶层/嵌套扩展、必填/null/错型/权限/引用、重复键/超限、失败后身份/历史/个人/退出和修复恢复；源导出或全局 Json 开关不能替代这些结果。Android 工具、报告与设备范围由其仓库维护。

### Direct MCP 与真机预设

[Direct MCP 参考](direct-mcp-tool-governance.md#验证入口与边界)维护定向入口。合同/Hub/HTTP 覆盖 ALL/ALLOWLIST、非空限制、完整 Tool JCS、binding、disabled-source 与发现失败；Console/browser 验证保存重开、Preview/Publish、权限保全、搜索/分页和宽窄屏。AUTO/REQUIRE_CONFIRMATION 的原生执行仍需实际消费者验证。

`scripts/real-device-preset.test.mjs`、`scripts/admin-build-snapshot.test.mjs` 验证 preset migration/数据保全、完整 opening、server-owned discovery 回传、显式 ALL、build identity 和安全失败诊断。未变化投影不额外发布；失败结果不能借用前一轮输出。`device:real` 的启动与凭据操作统一见[开发说明](development.md#actual-android--admin-development-environment)。

真实供应商 lane 单独记录实际调用和失败：ACTIVE、ready、连通不证明凭据/授权或模型可用；关联 browser-authored release/hash、设备 Applied 与实际响应。所有凭据和原始私人正文排除在证据外。

## 发布历史与上游删除专项验证

语义以 Control Protocol §16 及 Hub/Admin Testing Spec 为准；具体迁移机制见[数据库迁移](database-migrations.md#发布历史迁移)，操作流程见[Spark 部署手册](s02-preview-deployment.md#101-发布历史清理与连接删除)。

从 `backend/` 执行：

```text
go test ./migrations ./internal/hub/capability ./internal/hub/httpapi ./internal/hub/upstream ./internal/hub/budget ./internal/hub/usage -count=1
```

`capability/history_test.go` 验证保护项、过期预览、半开时间范围、有界批次、规则并集/关闭及自动清理不改变运行状态；HTTP 测试覆盖 Cookie/CSRF、规则修订、引用诊断和清理后独立删除。预算/用量回归验证清理前准入名称固化、所有资源类型历史展示及迟到结算/重放；历史重发分别验证新请求拒绝已清理源与已完成命令的幂等返回。

迁移必须测试真实 SQL 结构：`release_history_upgrade_test.go` 覆盖原名称与 generation 高水位；`upstream_history_upgrade_test.go` 从带历史准入、结算、待核对、用量和定价的旧库升级，调用真实删除服务，再逐列比对历史值、自增高水位和其他约束。Ent 临时建库不包含全部历史 DDL 外键，不能替代该升级测试。

从 Core 根目录执行受影响 UI 回归：

```text
pnpm -C console test --run src/components/ReleaseHistoryTools.test.ts src/pages/ReleasesPage.test.ts src/pages/UpstreamsPage.test.ts
```

测试预览/确认、规则与审计、列表选择和失败后的重新预览；真实浏览器还须使用生产 SPA 核对实际候选、清理后保留记录、引用解除及独立删除。现场证据绑定包摘要，区分管理员并发新发布/设备正常 Applied 更新和清理副作用，不宣称这些专项结果已覆盖所有 Admin/Android 门禁。

## 远程工作区专项验证

工作区机制见[实现参考](remote-workspace-implementation.md)。Go 检查响应丢失/UNKNOWN 恢复、身份/绑定/config、租约、XML/路径/条件、历史迁移及预算；生产 Console 检查配置、生命周期、文本字节往返/冲突和资源查询在途切换。

从 Core 根目录运行真实隔离服务 lane：

```text
pnpm -C console build
node scripts/workspace-integration.mjs --config <local-config.json> --output <new-evidence-directory>
```

私有配置包含 `adminOrigin`、`mcpOrigin`、`davOrigin`、`managementTokenFile`、`releaseIdentity`、`imageIdentity`。Token 单独保护；后两项是测试源码/镜像定位，不发送给服务配置 API。脚本只允许 loopback，建立独立临时 Core 库、UUID 用户与远端空间，验证双用户 MCP/DAV、预算、64 MiB、取消、重启与删除。成功后删除本轮空间并停止进程，保留 DB/日志/脱敏 evidence；失败写入不自动重放，保留未知远端目标供核查。

`--ui-only --output <directory>` 建立未配置环境供真实网页操作；`--keep-ui` 在通过后保留审查用户。登录资料写私有 `ui-env.json`，不输出到终端。脚本不安装第三方服务，不代替生产版本核验、浏览器或 Android 验收；完整 S1 compute/storage 计量仍按阶段权威推进。间歇传输失败记录与未确认根因见[历史证据和风险](s0-execution-progress.md#保留的失败边界)。

## CI、TDD 与变更证据

### 当前 CI

`.github/workflows/ci-gate.yml` 触发 PR to main / push to main；工作 jobs 为 static-contract、backend-test、console-test、system-test，汇总 ci-gate。静态 job 执行 tooling、fmt-check、contract，system 使用 smoke；不生成、不比较 drift、不跑浏览器或外部资格。

合并前观察最新 PR commit 的实际 check/log。较早 Green 不覆盖新 commit；ci-gate Green 不证明生成一致或 C6/C7，生成后须检查并提交预期产物。

### TDD 与变更证据

1. 最小有意义测试先失败，确认失败来自目标行为，而非无关配置/环境。
2. 最小实现使测试通过，保留该测试；同步合同、生成材料和 migrations。
3. 重构不弱化约束，执行受影响 suite/真实边界，并观察最新结果后报告 Green。

本地记录命令与实际输出；GitHub-only 先开 Draft PR，让 Red 提交触发 Actions并检查具体失败，再提交 Green、观察最新 gate。只提交 Red 不观察、静态阅读推断失败、同一提交加入测试和实现却宣称观察过 Red，均不足以证明过程。

文档/注释调整无需人工 Red。纯重构依赖有效现有回归；behavior 或 schema 变化仍需有意义的失败和升级保全验证。PR 使用已有模板，说明架构影响、Red/Green、执行层次、合同/持久化/操作影响与剩余缺口。

## 候选与外部资格

`make s01-candidate-test`、`make s01-browser-candidate` 是显式 CAP 入口；完整 C6/C7 还包括架构要求的外部资格、固定资源/合同、干净源码 rebuild/replay 与单独 finalize，完整顺序由[发布文档](release.md#s01-candidatefreeze)维护。当前 CAP compiler pins v5 与 CAP-C0-010 的 shared v4/v5/default/opening 检查，不能作为 ERX 设备结果生产者。

真实 Adapter qualification 要在同一固定版本/config/profile 的运行中覆盖全部四项及 required forwarded Usage、transport、上游身份；不能拼接部分探测、把 LEVEL_0 或 deterministic Adapter 写成已认证。历史 FAILED 报告只作定位，当前晋级必须重新取得实际资格。

证据保存固定源码/构建/合同/fixture、设备/APK 或浏览器、命令、场景、时间、失败和 teardown；不含 credentials、Secret、生产对话或请求正文。writer/replay 拒绝 dirty、stale、缺失和 hash mismatch，输出新文件不覆盖旧证据。manifest 的 PASS 字段须能追到实际源/build/log/artifact。
