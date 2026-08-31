# S0 Platform Core 当前实现状态

> 状态日期：2026-08-31
> Architecture authority：[阶段阅读清单](../../measix-architecture/docs/measix-stage-document-index.md)及 owning contracts
> 本轮实现修复起点：core `7e0c98c`（agent/s0-platform-core）；architecture `345d206`（main）。
> 本文件为唯一 living implementation/stage status；[alignment audit](architecture-alignment-audit.md)保留修复前的 dated evidence。

## 当前判断

本轮直接审查并修复 S0–S0.2 既有实现，没有新增 Gateway、管理进程、消息队列或平行状态机。可执行合同、迁移、生成物、回归测试和运维/开发文档已随修复同步。当前 compiler 仍输出 Snapshot v2；组件/确定性系统通过不等于 S0.2 Freeze。

## 修复与职责收拢

| 范围 / 原审查项 | 当前实现 |
| --- | --- |
| Session / A01 | Android 七天 rolling idle，Refresh 单事务旋转 + Idempotency-Key + 两分钟加密持久恢复；同命令丢响应可重取，过期/禁用/撤销不返回凭据。Logout channel 隔离；ACTIVE 同安装重新 Enrollment 保留 Device ID、撤销旧 Session；Enable 不复活旧凭据 |
| Discovery / A02 | 只返回同源 Client/Runtime paths；移除无效 absolute-base / refresh-TTL 配置。Enrollment 201、deviceName 持久化、idle expiry、严格 JSON/Problem/error 分类和 JWT exp 检查对齐 |
| Snapshot / A03 | 空 Seed 合法，作者顺序不排序；Starter wire 按自身 stable ID canonical，显示顺序另按 sortOrder/ID。Release read/diff 遇损坏 JSON 显式报错 |
| Feed / A04–A05 | Deployment 持久 timezone/revision；Publish/Withdraw 事务推进，Draft 不推进公开 revision；内容/revision/ETag 一致读，查询及跨午夜纳入 ETag，自然日 DST，camelCase HTTP |
| Admin / A06、A08 | Assistant/Seed/Starter typed editor 共用 DraftStore/Save/Validate/Preview/Publish；Review 和 Preview 覆盖新增对象。列表分页与配置选择器取全；Activation 按命令目标/内容保留重试 key；vue-tsc 覆盖模板并修复旧类型/复制按钮 |
| Usage / A07–A08 | 已认证未命中拒绝也写一条 unforwarded fact，缺失归属留空而非伪造；列表 keyset cursor、时间/用户/资源/状态/完整度与汇总一致，混合 semantic completeness 按整个 request 聚合；跨用户金额不混入，可得成本小计不冒充完整 |
| Runtime control | Apply Upstream 持久 targetRevision，丢 ACK 可恢复；已提交 descriptor 原样 rehydrate，不用可变事实在旧 revision 内重编译。既有 reconciler 将 Session revoke intent 经普通 Activation 收敛至 Relay，无额外 outbox/daemon |
| Hosting / A09–A10 | Hub main 可配置生产 SPA 目录；private listen 默认 loopback。HTTP bounded drain 后 cancel/close/join，Relay server error 仍执行 cleanup/final flush。公开 health 不再读取完整 Admin diagnostics |
| Persistence / A11–A12 | SQLite PRAGMA 每连接生效；4 份增量迁移，不改已发布 SQL 语义。真实 SQL empty/upgrade 测试替代 no-op upgrade fixture。Check 校验当前 Ent 表/列 + integrity/FK；backup 双目标独占保护并检查副本；revision 是 binary expected schema，不伪装 Atlas history |
| Metering / System | spool retry 使用时间比较；负 ACK/trailing JSON 不删除 spool；append 失败保留稳定事件诊断，sticky degraded 不因后续写成功而掩盖丢失。System 透传 spool/pending/oldest age，缺失为未知 |
| Tooling / A14–A15 | dev setup 保留既有 keys/password，bootstrap --if-empty；strict dev migration checksum/事务/有序 ledger。跨平台 Node 单一生成/静态检查 owner、Make 工作目录/失败传播修正；证据验证 source/build/contracts/artifacts/scenarios，四 profile 必须同次真实验证且具身份/usage 信息；历史 manifest 禁止覆盖 |

新增语义只放在 architecture Control Protocol：refresh recovery、re-enrollment/session deny 收敛和 Usage 筛选口径。具体类/函数/迁移/配置/命令仍归 core；没有在架构仓库复制实现清单。

## 本轮验证

以下是本次工作树实现的回归证据，不是独立 clean-source Freeze：

- 完整 `go test -p 1 ./... -count=1`：退出码 0；包含新 CLI bootstrap/static hosting、Session、Feed、Usage、持久化和 shutdown 回归。
- `go vet ./...`：通过。
- `go test -tags=smoke ./test/system/scenarios/ -count=1 -timeout 5m`：通过。
- `go test -p 1 -tags=candidate ./test/system/scenarios/ -count=1 -timeout 15m`：退出码 0，约 183 秒；未使用 -short。真实 Hub/Relay/Adapter 测试仍不是浏览器或真实外部 Adapter。
- Console `vue-tsc`/E2E TypeScript、production build：通过；Vitest 15 文件、70 测试通过。
- `node scripts/e2e-harness.mjs`：退出码 0。同一 fresh runtime 的 Browser authoring/Publish（含 Assistant/Seed/Starter 编辑与 Preview 顺序校验）→ Model/TTS/ASR/MCP traffic → usage ingest → Browser Usage/System → topology security 全部通过；三个 Playwright 场景，无 retry。尚未覆盖真实 Android/Portal 的完整 ERX。
- Browser 首次失败为 Review 缺失翻译键与宽泛 locator 匹配两个节点；第二次为新增测试把 Quasar native input 的 data-cy 当作父容器。分别修复翻译/精确选择器与原生 input locator 后全链通过，未扩大 timeout。失败 trace/截图/结果保留在 ignored `.artifacts/review-browser-failure-20260831` 和 `.artifacts/experience-locator-failure-20260831`。这些合成凭据测试 artifact 不自动上传。
- Node evidence/tooling regression tests、脚本语法、生成链、gofmt：通过。
- 关键 Red→Green 包括 refresh deadline、Feed revision、canonical order、Usage 串用户/完整度、no-route 计量、invalid ACK、SQLite 换连接、migration 部分失败、shutdown cancel、丢 ACK/restart/security、typed editor、命令重试 scope 和公开 health 职责。新增配置入口测试为补充验证，不伪称观察过旧实现 Red。

## 尚未满足的交付 gate

1. S0.2 Portal Web Session 的签发/兑换/受限 Cookie/CSRF/母 Session 绑定合同与实现仍未完成；真实 Android 的安全 refresh 落盘、Realm/Experience/Portal consumer 和完整 ERX gate 未验收。
2. 当前 CAP manifest compiler 明确仅支持 S0.1，拒绝将 Snapshot v2 伪装为 v1；独立 clean-source checkout/rebuild/replay 未实现。runtime-only replay 不能 finalize CAP-C7-002，不声称 `make freeze-gate` 已可完成当前 S0.2 Freeze。
3. 本机未执行 Atlas CLI 全 gate（未安装 Atlas/Make）与 race gate（CGO_ENABLED=0，PATH 无 C 编译器）。真实 SQL/checksum/升级测试不能替代这两条独立门禁。
4. 真实外部 Adapter qualification、Android/Portal、production TLS/ingress、isolated production restore、supervisor/log collector 验收未执行。脚本静态修正不等于真实 qualification VERIFIED。
5. S0.3 Gateway binary/OpenAPI/v3 catalog/surface/三进程 harness、原生 service units/日志规范与收集仍按 S0.3 实现，不在本轮越级开发。默认开启/服务端 REQUIRED 的两工具是 Gateway 的原子能力对；Direct Managed MCP 仍保持 Assistant-bound，不设 fallback。

Session Logout 204 表示 Hub durable revoke，不是 Relay 收紧 ACK；现存 Access Token 的短期有效窗口已明确。内部双向 service token 复用、生产 TLS/服务包、完整 applied migration history/索引 attestation 等边界如实记录在 [operations](operations.md) 与 [database migrations](database-migrations.md)，不隐瞒为已部署能力。

## 历史 evidence

保留的 `docs/s0-freeze-manifest.json` 声明 architecture `cc60f8f540d309f2b73228094c8b9cd1b0b0a60f`、core `a6075bc0afd78fa86d77e1a520f838c954c9adfa`、Snapshot v1。本轮没有改写该 JSON，也没有重放/完整验证其外部 artifact 链；它不覆盖当前 v2 或 S0.3/S0.4。新 candidate 只写独占的 artifact 路径。
