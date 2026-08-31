# Playwright Browser E2E 实施与排障

本文只记录当前工程入口和诊断方法；CAP/ERX/ETG 行为要求由 architecture Testing Specs 定义。当前阶段判断见 [execution progress](s0-execution-progress.md)，本轮源码审查见 [alignment audit](architecture-alignment-audit.md)。

## 1. 当前执行入口

在 CI-compatible POSIX 环境从仓库根执行 `make s01-browser-candidate`（`make console-e2e` 是 alias）。等价的主要步骤是先 `pnpm -C console build`，再 `node scripts/e2e-harness.mjs`。先安装仓库锁定版本的依赖及匹配 Playwright 浏览器；直接运行 Playwright 默认 URL 不会自动创建 Hub/Relay 测试环境。

Node harness 使用临时 DB/keys/ports、真实 Hub/Relay、deterministic Adapter 和 production SPA proxy；Go `backend/test/system/` 是另一套测试 orchestration。两者应共享合同和证据规则，不声称物理上是一套环境。

当前 golden path 分为：

1. `golden-path-authoring.spec.ts`：浏览器创建/编辑/发布业务配置。
2. orchestrator/Test Client：在同一环境产生真实 runtime traffic。
3. `golden-path-usage.spec.ts`：浏览器确认 Usage/System 结果。

`topology-security.spec.ts` 执行额外拓扑场景；旧 `golden-path.spec.ts` 在配置中被忽略，不是当前主入口。不要把三步拆到互不相关的数据库后合并成一次闭环证据。

## 2. 浏览器配置事实与限制

`console/playwright.config.ts` 当前固定一个 worker、零 retry，使用 list/HTML/JSON reporters，HTML `open: never`。JSON 默认写 `.artifacts/e2e-playwright.json`，可由 `PLAYWRIGHT_JSON_OUTPUT_FILE` 指定；harness 会管理分阶段证据。

当前 trace=`retain-on-failure`，零 retry 时也保留失败 trace。保留截图/视频不能替代响应/状态/来源证据。

配置已移除显式禁用 sandbox/isolation 和忽略证书错误的 launch args；采用框架默认启动行为，不声称已验证 OS sandbox。HTTP loopback harness 仍不证明生产 TLS/ingress。确需诊断例外时必须与安全验收 lane 分开。

## 3. 定位失败，不猜根因

先记录 exact commit、操作系统、浏览器/Playwright 版本、入口命令、失败步骤和退出码，再按边界检查：

| 现象 | 首先核对 |
| --- | --- |
| 导航失败/ERR_ABORTED/reset | 实际端口、进程存活、server/proxy 是否收到请求、HTTP response/redirect、资源加载和 teardown 时序 |
| SPA 页面出现但 API 失败 | 同源 proxy 路由、Hub public/private 分离、cookie/CSRF、真实响应 Problem |
| Publish 后状态未收敛 | Activation 状态、Hub reconcile、Relay desired/applied revision、服务认证 |
| Usage 页面没有新请求 | 同环境 traffic 是否产生，Relay spool/Hub private usage URL、ingest/cost 状态 |
| 浏览器进程未退出 | reporter 是否启动 server、子进程/worker 句柄、失败分支 teardown |
| UI 找不到元素 | 实际 DOM、accessible role/name、locale、选中对象和业务状态，不先扩大 timeout |

历史 ERR_ABORTED 不能证明 Windows headless shell 普遍存在 loopback 缺陷，更不能据此关闭/终止安全软件。若怀疑环境问题，先用相同版本、最小本地 server 和可重复诊断隔离变量；没有证据就标记“原因未确定”。

同步阻塞调用若与 HTTP server 共用 Node event loop，确实会阻止它处理请求；但应先核对当前调用链和 worker/process 边界。当前存在 `_server-worker.mjs`，旧故障分析不能直接当作当前根因。

## 4. 稳定断言与数据边界

优先使用唯一的 accessible role/name 或稳定 test ID；状态匹配使用精确文本，例如 ACTIVE 不能用会同时匹配 INACTIVE 的宽泛正则。不要用 `.first()` 掩盖元素选择歧义。

异步业务状态使用有 deadline 的 polling/assertion；固定长 sleep、盲目重试或“延长 timeout 直到通过”不是可靠性修复。确需测试时间窗口时说明测试语义和上限。

业务对象通过被测 UI/API 工作流创建，不直写 DB、内部 Relay control 或用 `page.route('/api/**')` 模拟 Hub 来声明 T4.1 Green。故障注入必须明确在外部 Adapter/网络边界，不掩盖 MEASIX 被测组件。

## 5. 证据、清理与安全

失败也要等待/终止自己创建的子进程与 worker，保存必要诊断；只清理本次明确拥有的临时目录，不能按进程名批量杀系统进程或删除开发数据库。

测试只用合成身份/密钥。trace、HAR、截图、DOM、日志和 JSON 均可能携带 cookie、token、表单 Secret 或请求正文；留存前审查/脱敏，不上传生产或真实用户数据。保留失败事实，不手改结果 JSON 为 PASS。

浏览器 Green 只是对应场景在该 composition 的证据；默认 CI、旧截图、server library static-host 单测都不是当前完整 C6/C7/Freeze。完整证据链要求见 [release](release.md)。
