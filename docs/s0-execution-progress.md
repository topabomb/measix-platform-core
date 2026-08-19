# S0 Platform Core 开发计划与当前进展

> 架构权威：`topabomb/measix-architecture` 默认分支。  
> 当前架构基线：`c2f517b52d84b1b4de5e5bab46a7a15d936a4e32`（2026-08-19）。  
> 本轮范围：仅 `measix-platform-core`；不修改 `rikkahub_mcp` 与 `weero-agent-space`。

## 执行规则

实现只以当前 `measix-architecture` 默认分支为权威。当前 S0 基线采用 Control Hub / Runtime Relay、SQLite + Ent + Atlas，以及 `PUT /internal/v1/control/state` 的完整 desired-state apply；不实现旧 prepare/barrier/commit 协议。

行为开发执行 Red → Green → Refactor。面向开发者、评审者和运维人员的说明文档优先使用中文；只有明确给 AI/Agent 使用的执行契约（例如 `AGENTS.md`）以及代码/机器配置按工程需要使用英文。

## 阶段计划与状态

| 阶段 | 本仓库实施范围 | 状态 | 验收证据 |
|---|---|---|---|
| I0 工程与可执行契约 | 4 OpenAPI、fixtures、codegen、platformid、Hub/Relay health、SQLite/Ent/Atlas、Quasar shell、T0/T1/T2/CI | 已完成 | Red：commit `28504cef...` / run `32210208203`；生成链 run `32211905037` 已通过 Go module、4×OpenAPI、Ent、TS、Atlas hash；T0 contract tests `SYS-I0-001/002/003` + `TestS0OpenAPISurfacesValidate` + `TestSnapshotAndRuntimeControlGoldenHashes` 本地通过 |
| I1 Identity & Enrollment | Hub 身份/Enrollment/Session/Admin auth、Admin User/Enrollment/Device | 已完成 | commit `5fe7591 feat: 完成 Users Enrollment 与安全变更流程`；`TestSYSI1001IdentityHTTPClosedLoop`、`TestI1IdentityEnrollmentRefreshAndRevoke`、`TestAdminSessionCookieAndCSRF`、`TestHUBID002/003/005` 本地通过 |
| I2 Draft & Snapshot | Upstream/Secret、typed Draft aggregate、validation、deterministic Snapshot/ETag、Admin workflow | 已完成 | `TestHUBCAP001DraftOptimisticConcurrencyAndSaveDoesNotActivate`、`TestHUBUPS003SecretVersionsAreAppendOnlyEncryptedAndReferencedPrecisely`、`TestHUBCAP006SnapshotDeterministicAndClientSafe`、`TestHUBCAP008StagedReleaseIsImmutable` 本地通过；Admin Upstream 列表分页（`TestAdminContractExposesUpstreamList`）已 Green |
| I3 Relay & Publish | full-state atomic apply/status、JWT/admission/route/proxy、Activation/reconcile/republish | 已完成 | `TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck`、`TestI3ControlApplyAndRuntimeAdmission`、`TestI3ControlRevisionConflictKeepsCurrentState`、`TestI3ReconcileFinalizesPublishWhenRelayAppliedButAckWasLost`、`TestI3ReconcileDoesNotBlindlyOverwriteUnexpectedNewerRelay`、`TestI3ActivationDescriptorDoesNotContainCredentialMaterial` 本地通过 |
| I4 Relay transport completion | Model/SSE、TTS binary、ASR multipart、MCP Streamable HTTP、cancel/limits；Android 源码不在本轮 | Relay 侧已完成 | `TestRLYI4TransportsStreamWithoutProtocolTranslation`（TTS/ASR/MCP 子用例）、`TestRLYI4CancellationPropagatesToUpstream`、`TestRLYI4InFlightRequestKeepsCapturedControlState`、`TestRLYROUTE005*`、`TestRLYHDRCredentialHeaderCannotRestoreReservedHeaders` 本地通过；Android 源码属 `rikkahub_mcp` 仓库，不在本轮 |
| I5 Metering/Admin/Hardening | Relay SQLite spool、Hub ingest/dedupe/pricing、Admin status/usage、migration/backup/restart/security/load harness | 核心已完成 | `TestRLYI5RuntimeWritesCapturedRequestUsage`、`TestRLYSPHubAckDeletesOnlyAcknowledgedBatch`、`TestRLYSPHubOutageKeepsRowsAndRecordsBackoff`、`TestRLYSPPoison422IsolatedWithoutDroppingGoodRows`、`TestHUBI5RequestUsageBatchIsIdempotent`、`TestHUBI5SemanticUsageDedupeAndCompleteness`、`TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic`、`TestHUBI5UsageIngestHTTPContract`、`TestI5SecurityDisableIsDenyFirstAndEnableIsAllowLast`、`TestI5DeviceRevokeIsAppliedToRelay`、`TestHUBDBIntegrityAndBackupRestore` 本地通过；T4 真实 Adapter/Android emulator/浏览器 E2E 与 target VM 资源验证留待 RC |

## 本轮边界与 S0 RC 关系

本轮实现所有不依赖 Android 源码修改或 Agent Space 修改的 platform-core 需求。以下跨仓库/外部输入不修改，也不会伪报通过：Android instrumentation/Emulator、`weero-agent-space`、真实外部 Adapter 凭据、最终固定 Android commit 的 T4。platform-core 保留对应接口、fixture、harness 和 manifest 字段。

## 当前进展记录

- 2026-08-19：重新读取架构仓库当前 `main`，废弃旧 Control Server / Runtime Gateway / barrier-saga 假设。
- 2026-08-19：确认技术栈为 Go 1.26.x、SQLite + Ent + Atlas、Vue 3 + Quasar、OpenAPI 3.0.3。
- 2026-08-19：建立实施分支和 Draft PR #1。
- 2026-08-19：完成真实 TDD Red：GitHub Actions 在 `platformid` 行为断言处失败，而非工具链/配置失败。
- 2026-08-19：I0 executable source 已落库；4 份 OpenAPI 与 canonical fixtures 可解析。
- 2026-08-19：GitHub Actions 已实际完成 `go mod tidy → 4×oapi-codegen → Ent → pnpm/openapi-typescript → Atlas hash`；Quasar 3 HTML 入口已修正。
- 2026-08-19：T0 contract、`platformid`、health、Hub app、generated wire 已进入 Green；当前继续收敛 Ent 生成后依赖与完整 T0/T1/T2 Gate。
- 2026-08-19：I1 完成（commit `5fe7591`），Identity/Enrollment/Session/Admin auth 全链 T2 通过。
- 2026-08-19：I2 完成，Upstream/Secret（加密版本化）、Draft（乐观并发）、Snapshot（确定性 hash）、Staged Release（不可变）全部 T2 通过。
- 2026-08-19：I3 完成，Publish intent 持久化 → Relay ACK → finalize、reconcile、republish、security change（deny-first/allow-last）全部 T2/T3 通过。
- 2026-08-19：I4 Relay 侧完成，SSE/binary/multipart/MCP Streamable HTTP transport、cancellation、in-flight capture、routing security 全部 T2 通过。
- 2026-08-19：I5 核心完成，Relay SQLite spool（at-least-once + Hub requestId 去重 + poison row 隔离）、Hub ingest、pricing、usage query、security revoke、DB backup/restore 全部 T2 通过。
- 2026-08-19：Admin Upstream 列表分页 TDD 完成（Red `TestAdminContractExposesUpstreamList` → Green `GET /api/admin/v1/upstreams` + `UpstreamPage`），OpenAPI + 生成代码 + handler 一致。
- 2026-08-19：Node 升级至 v24.19.0（满足架构 Node 24 LTS 要求），pnpm 11.0.0 安装；前端 `pnpm typecheck && pnpm test && pnpm build` 本地全通过（3 files / 8 tests）。
- 2026-08-19：新增根级 `.gitignore`（Go/Node/OpenAPI/Atlas/编辑器）和 `package.json`（monorepo npm scripts 编排：`npm run dev/build/test/test:backend/test:console/contract/generate/drift/ci`）。
- 2026-08-19：本地运行链路打通：`npm run setup`（密钥生成 + 迁移 + admin bootstrap）→ `npm start`（hub:8080 + relay:8090/8091 + console:9000）→ admin 登录 200 + cookie + CSRF 验证通过；`backend/cmd/devmigrate` 为本地 dev-only 迁移 applier（CI 仍用真实 atlas replay）。
- 2026-08-19：修复前端白屏回归：`LoginPage` 在 layout 外使用裸 `QPage` → 改为自带 `QLayout→QPageContainer→QPage`；TDD Red→Green 补 `LoginPage.test.ts` 回归测试（Red：QLayout 缺失 + inputs 渲染 0；Green：2 tests passed）。
- 2026-08-19：修复图标缺失：安装 `@quasar/extras` 并配置 `quasar.config.ts` `extras: ['material-icons']`；验证 build 产物 CSS 含 material-icons font-face + woff/woff2 字体。升级 `@quasar/app-vite` 3.3.0→3.7.0（Windows 路径分隔符 bug）。多语言切换：S0 架构技术栈（admin-console §2.1）未包含 i18n，验收清单亦无此要求，不私自引入 vue-i18n（需先经 measix-architecture Implementation Decision 批准）。
- 2026-08-19：GitHub Actions 清理：删除 `apply-upstream-list.yml`（GitHub-only 模式遗留的自动 apply-patch+commit+push workflow，违反 docs/testing.md §8 非约定 gate 且含 contents:write）与 `.github/patches/`；删除 `dev-export.yml`（GitHub-only 遗留源码导出，非 gate）。保留唯一 PR gate `ci-gate.yml`（static-contract → backend-test + console-test → ci-gate aggregate，符合约定）。

## 本地验证状态（2026-08-19）

环境：Node v24.19.0、pnpm 11.0.0、Go 1.26.x。

| 验证项 | 状态 | 说明 |
|---|---|---|
| `go build ./...` | 通过 | 全部 Go 包可编译 |
| `go vet ./...` | 通过 | 无静态问题 |
| `go test ./... -count=1` | 通过 | 全部后端测试 Green，无 skip |
| Go codegen drift（admin） | 通过 | `oapi-codegen` 重新生成 `admin.gen.go` 与工作树一致 |
| TS codegen drift（generated.ts） | 通过 | `pnpm generate:api` 输出与工作树一致 |
| Contract tests（T0） | 通过 | `SYS-I0-001/002/003` + OpenAPI validate + golden hashes + ListUpstreams |
| 前端 `pnpm typecheck` | 通过 | `quasar prepare && tsc --noEmit` 退出码 0 |
| 前端 `pnpm test` | 通过 | Vitest 4 files / 10 tests 全通过（client/workflow/stores/LoginPage 回归） |
| 前端 `pnpm build` | 通过 | Quasar SPA 构建成功；产物含 material-icons font-face + woff/woff2 |
| LoginPage 白屏回归测试 | Red→Green | Red（stash 修复后）：QLayout 缺失 + inputs=0；Green（恢复修复）：2 passed |
| 本地运行链路 | 通过 | setup → hub+relay+console 启动 → admin login 200 + Set-Cookie + csrfToken |
| Atlas migration replay | 未执行 | 本地未安装 `atlas` CLI（Go 1.26 兼容性问题）；迁移 SQL 与 `atlas.sum` 已落库，CI 用真实 atlas 验证 |
| Go race detector | 未执行 | 本地 `CGO_ENABLED=0`，`-race` 需要 cgo；Makefile 指定 race 测试包为 `platformid/health/sqliteutil/metering` |

### 前端实现覆盖度

| 页面/模块 | 状态 | 说明 |
|---|---|---|
| `OverviewPage` | 已实现 | 平台总览、健康状态、平台 ID 展示 |
| `LoginPage` | 已实现 | 自带 QLayout；白屏回归测试覆盖 |
| `UsersPage` | 已实现 | 用户列表、Enrollment/Device 管理 |
| `UpstreamsPage` | 占位 | I5 前端剩余：Upstream 列表/编辑/apply（后端 API 已就绪） |
| `ResourcesPage` | 占位 | I5 前端剩余：Draft 资源编辑器 |
| `ReleasesPage` | 占位 | I5 前端剩余：Release 列表/republish/activation 恢复 |
| `UsagePage` | 占位 | I5 前端剩余：Usage 查询（后端 ListUsageRequests/GetUsageRequest/UsageSummary 已就绪） |
| `SystemPage` | 占位 | I5 前端剩余：System 诊断（后端 SystemHealth/SystemStatus 已就绪） |
| `api/client.ts` | 已实现 | OpenAPI 类型化客户端、CSRF/cookie 处理 |
| `stores/session.ts` | 已实现 | Pinia session store |
| `stores/{draft,activation,operationalApply}` | 已实现 | 4 tests 覆盖 stale 409/幂等重试/candidate≠active |
| 路由守卫 | 已实现 | 未登录重定向 `/login` |

### 前端测试 vs 架构 admin-console §19.5 清单

| 架构要求 | 现状 |
|---|---|
| typed Problem mapping | ✅ client.test.ts + workflow.test.ts |
| draft stale 409 | ✅ workflows.test.ts DraftStore |
| Publish 202 + 幂等重试 | ✅ workflows.test.ts ActivationStore |
| Upstream candidate≠active | ✅ workflows.test.ts OperationalApplyStore |
| LoginPage 可独立渲染（本轮回归） | ✅ LoginPage.test.ts |
| cursor list helpers | ❌ 缺（UpstreamsPage 实现时补） |
| activation refresh recovery（activationId 恢复） | ⚠️ 部分（polling 有，刷新恢复场景未覆盖） |
| security revoke pending Relay enforcement | ❌ 缺（UserDetail 操作实现时补） |
| secret 不持久化 | ❌ 缺（Upstream/Secret 表单实现时补） |
| Unknown/Partial usage 展示 | ❌ 缺（UsagePage 实现时补） |

## 待提交变更（工作树）

当前分支 `agent/s0-platform-core` 工作树有以下未提交变更：

| 文件 | 变更 | 验证 |
|---|---|---|
| `api/admin/admin.openapi.yaml` | 新增 `GET /api/admin/v1/upstreams`（`listUpstreams`）+ `UpstreamPage` schema | OpenAPI validate 通过 |
| `backend/internal/wire/adminapi/admin.gen.go` | 由 OpenAPI 重新生成（`ListUpstreamsParams`、`UpstreamPage`） | codegen drift 校验通过 |
| `console/src/api/generated.ts` | 由 OpenAPI 重新生成（`listUpstreams` operation、`UpstreamPage` interface） | `pnpm generate:api` drift 校验通过 |
| `backend/internal/hub/httpapi/admin_upstream.go` | `ListUpstreams` 接受 `ListUpstreamsParams`、返回 `UpstreamPage` | `go test ./internal/hub/httpapi` 通过 |
| `backend/internal/relay/runtime_metering_test.go` | 失败诊断增加 response body 输出 | `go test ./internal/relay` 通过 |
| `.gitignore`（新增） | 根级忽略：Go 二进制/coverage、Node `node_modules`/`dist`/`.quasar`、Atlas 运行时、本地 DB/密钥（`.secrets/`、`.data/`）、编辑器缓存 | `git check-ignore` 验证 node_modules/dist/.quasar 被忽略，pnpm-lock.yaml/package.json 未被忽略 |
| `package.json` + `package-lock.json`（新增） | 根级 monorepo 编排：`setup/start/start:hub/start:relay/start:console/test/contract/generate/drift/ci` 等统一命令 | `npm run test`（后端+前端）、`npm run contract` 验证通过 |
| `scripts/dev-setup.mjs`（新增） | 一键 bootstrap：密钥生成、devmigrate、admin bootstrap | `npm run setup` 全链路验证 |
| `backend/cmd/devmigrate`（新增） | dev-only 迁移 applier（本地 atlas CLI 不可用时的替代；CI 仍用真实 atlas replay） | `npm run setup` 验证 |
| `console/quasar.config.ts` | dev server proxy（架构推荐 `/api`→Hub）+ `extras: ['material-icons']` | build 产物含图标字体验证 |
| `console/src/pages/LoginPage.vue` | 白屏修复：自带 QLayout | TDD Red→Green |
| `console/src/pages/LoginPage.test.ts`（新增） | 白屏回归测试（QPage 必须有 QLayout 父级） | Red→Green 双向验证 |
| `console/vitest.config.ts`（新增） | Vitest 组件测试配置（plugin-vue + jsdom + quasar client build alias） | 10/10 tests 通过 |
| `console/package.json` + `pnpm-lock.yaml` | +`@quasar/extras`、+`@vitejs/plugin-vue`/`jsdom`/`@types/node`（devDeps）、`@quasar/app-vite` 3.7.0 | typecheck/test/build 通过 |
| `.github/workflows/apply-upstream-list.yml`（删除） | 移除 GitHub-only 遗留的自动 apply-patch+commit+push 机制 | 违反 docs/testing.md §8 约定 |
| `.github/workflows/dev-export.yml`（删除） | 移除非 gate 的源码导出 workflow | GitHub-only 遗留工具 |
| `.github/patches/`（删除） | 配套 patch 一并清理 | Green 产物已直接落库 |
| `docs/development.md` | 新增 §11 本地运行命令（setup/start/devmigrate 说明） | 架构要求命令与实现同 PR 落档 |
| `docs/s0-execution-progress.md` | 更新阶段状态、本地验证状态、前端覆盖度、测试缺口 | 文档同步 |

## 剩余验证缺口（需 GitHub Actions / RC）

1. `atlas migrate apply` 从空库 replay + `atlas.sum` 完整性检查。
2. Go race tests（`platformid`、`health`、`sqliteutil`、`metering`）。
3. T3 跨组件：Hub↔Relay、Admin↔Hub 真实进程集成。
4. T4 system harness：deterministic Test Adapter 全场景 + 真实 Android emulator + 浏览器 E2E。
5. S0 RC：真实 Adapter qualification、target VM 资源预算、backup/restore 全链。
6. 前端 I5 剩余：Upstreams/Resources/Releases/Usage/System 五个页面实现 + 对应 §19.5 测试（cursor list、revoke pending、secret 不持久化、Unknown/Partial usage、activation 刷新恢复）。
7. 本轮推送后 ci-gate 在最新 commit 上 Green 的 CI 证据。
