# S0.1 Platform Core 进展

> Architecture authority：`topabomb/measix-architecture@6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b`  
> Implementation branch：`agent/s0-platform-core`  
> 当前阶段：**S0.1 Managed Capability Delivery**  
> 阶段阅读清单：`topabomb/measix-architecture/docs/measix-stage-document-index.md`

## 当前判断

S0 Core 基础已经建立：Identity/Enrollment、Draft/Release/Snapshot、RuntimeControlState、Relay admission/transport、Usage spool/ledger/pricing、Admin shell 和 deterministic runtime test foundations 均已有实现与回归测试。

**S0.1 尚未完成。** Client Control/Snapshot v1 仍是 pre-freeze；不得进入 S0.2 Android，也不得宣称 S0 Exit。

2026-08-20 的 architecture 更新把 Admin visual authoring / review / validation navigation 明确成 executable requirement，因此此前基于旧 architecture baseline 得出的 C1/C2/C3 Green 结论已失效。旧 Green 仍可作为底层回归证据，但不是当前 checkpoint completion evidence。

## C0–C7 当前状态

| Checkpoint | 状态 | 当前结论 |
|---|---|---|
| C0 Contract Audit & Freeze Preparation | ✅ Green | required profile 的 OpenAPI/fixtures/generated types/backend validation 已基本闭合；继续作为后续回归基线 |
| C1 Upstream Operational | ✅ Green | Create/Test/Apply 与 typed UpstreamConfig 已存在；existing candidate edit、Secret replace、save/discard/reset、409 conflict handling、candidate vs active revision 显示、refresh recovery 与 apply-failure candidate preservation 均已实现 |
| C2 Managed Resource Editor | ✅ Green | Provider/Model/TTS/ASR/MCP/Policy 基础编辑与 binding 已存在；collection → selected editor/detail、Policy Default Model/TTS/ASR pickers（仅 enabled 资源可选）、dirty/clean state、field/resource validation navigation、relationship view（kind filter、click-through、missing binding 可操作错误、candidate != active、disabled/unverified/degraded 状态编码）均已完成 |
| C3 Snapshot Projection & Preview | ✅ Green | canonical preview backend 与基础 UI 已存在；Review & Publish 结构化审查面（Added/Changed/Removed + Policy changes + Runtime routing impact + Warnings）、Client Snapshot Preview（Providers/Models/TTS/ASR/MCP/Policy foldable detail + Client receives vs never receives 区分）、Publish progress（VALIDATING→STAGING_RELEASE→APPLYING_RUNTIME→FINALIZING→ACTIVE/FAILED 阶段显示 + recovery context）均已完成 |
| C4 Runtime Reference Profile | ✅ Green | deterministic Adapter/Test Client 已证明 Chat request/response+SSE、TTS binary、ASR multipart、MCP 及关键 Relay admission/transport 行为；这是 deterministic T2/T3 证据，不等于 real Adapter qualification |
| C5 Usage / Pricing / Observability | ✅ Green（component checkpoint） | filters、request detail、pricing、summary、Overview/System observability 已具备并有 component/backend 回归；仍须在 C6 Golden Path 中证明跨组件产品闭环 |
| C6 Browser + Hub + Relay Product/System E2E | 🔴 Not Green | 当前只有 bounded T3/system smoke；缺 production Admin real-browser T4.1 Golden Path、完整 Hub→Relay→Adapter→Usage 组合证明与 required recovery/security product scenarios |
| C7 Client Contract Freeze Gate | 🔴 Not Started | C6 未 Green；real Adapter qualification 未完成；没有有效 Freeze manifest |

## 当前有效证据如何解释

- `ci-gate` / backend / console / current system smoke：证明最新执行 SHA 对应的 T0–T3 deterministic baseline；**不证明 C6/C7/S0.1 Freeze**。
- `docs/s0-review-report.md`：旧 architecture baseline 下的历史审查/回归映射，只作 historical evidence，不再作为当前 S0.1 completion source。
- real Adapter qualification：必须按 architecture qualification spec 单独执行并形成证据；deterministic Adapter 不能替代。
- Browser T4.1：必须对 exact candidate SHA 显式执行 production SPA + real Hub + real Relay + deterministic Adapter/Test Client；默认 GitHub Actions 不执行该 gate。

## Freeze 状态

当前**不存在有效 S0.1 Freeze**。仓库不保留 placeholder/stale `docs/s0-freeze-manifest.json`；只有在 C6 required scenarios、real Adapter qualification 和 C7 contract evidence 全部 Green 后，才由 candidate verification 生成 machine-readable manifest。

有效 manifest 必须按 architecture S0.1 System Testing Spec / `docs/release.md` 固定 exact architecture/core/build/contract/adapter/scenario identities。任何旧 SHA 生成的 preliminary JSON 都不得继续叫 Freeze manifest。

## 下一步顺序

保持 `measix-s0-capability-delivery-implementation-decision.md` 的 C0–C7 顺序，不重新设计 Relay 或扩大 S0.1 scope：

1. **C6 产品闭环**：
   - 扩展 Browser T4.1 Golden Path 从当前 login→Overview→System→refresh 到完整 CAP-C6-001 流程
   - 创建一键 clean-environment T4.1 harness（自动 temp DB/Hub/Relay/Adapter/SPA）
   - 修复 CAP-C6-012 为真实浏览器刷新测试
   - 修复 CAP-C6-013 SQLite busy/transient 为真实锁竞争
2. **CAP scenario evidence mapping**：为 C0-C5 添加 Architecture Scenario → executable test → exact SHA result 映射
3. **Security required scenarios 补全**：expired JWT/wrong audience/disabled user/revoked device/session/management endpoint/Set-Cookie/redirect/Secret in browser/Snapshot Preview leak
4. **system-test vs s01-candidate-test 分层隔离**：当前两者执行同一 scenarios package，需真正隔离 bounded T3 vs 完整 Candidate Gate
5. **Freeze Manifest generator 修正**：真实 PASS/FAIL/startedAt/completedAt/C0-C5；不能在 tests 未通过时运行
6. 完成至少一套 required real Adapter qualification；
7. exact candidate 全部 Green 后生成 C7 Freeze manifest；
8. C7 之后才开始 S0.2 Android。

## 状态维护规则

本文件只维护**当前实现状态和剩余缺口**，不复制 architecture requirement 细节。Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。每次宣称 checkpoint Green，必须能指向当前 architecture baseline 与当前 implementation SHA 对应的 executable evidence。

## C6/C7 系统测试实现进展（追加）

> 以下为系统测试代码实现进展，**不改变上述 C6/C7 checkpoint 状态**。测试代码已实现并通过编译验证，但尚未作为 Green 证据执行。

### C6 系统测试代码

1. **HubEnv 测试环境**（`backend/test/system/harness/hub_env.go`）：
   - 创建隔离的 Hub+Relay 环境（独立 SQLite、Ed25519 密钥、relay service token）
   - 通过 `devmigrate` 应用迁移，`bootstrap-admin` 创建初始管理员
   - 构建 `control-hub` 和 `runtime-relay` 二进制
   - 管理进程生命周期（Start/Stop/Restart）
   - `WaitConvergence` 轮询直到 desired==applied controlRevision

2. **AdminClient**（`backend/test/system/harness/admin_client.go`）：
   - 管理 session cookie + CSRF token
   - 封装 GET/POST/PUT/DELETE

3. **Golden Path 测试**（`golden_path_test.go` + `golden_helpers.go`）：
   - `TestCAPC6001GoldenPath`：完整 Login→User→Secret→Upstream→Test→Apply→Resources→Validate→Preview→Publish→Activation→Relay→Usage 闭环
   - `TestCAPC6002TestClientFourCapabilities`：Test Client 通过 Client Control API 获取 Managed State/Snapshot，调用四种 runtime profiles
   - `TestCAPC6003UsageClosure`：四 resource kinds、filters、request detail、usage/cost completeness
   - `TestCAPC6004PublishNewGeneration`：N request→428/no-forward，client refresh snapshot，N+1 succeeds
   - `TestCAPC6011RelayRestart`：Relay 重启后 Hub rehydrate，fail-closed → READY
   - `TestCAPC6012RefreshDuringActivation`：浏览器刷新场景，同一 activation 恢复
   - `TestCAPC6014FullRestart`：Hub+Relay 全部重启，preserves active release/route/usage spool
   - `TestCAPC6015BackupRestore`：SQLite backup/restore preserves IDs/release/generation

4. **Recovery 场景测试**（`recovery_test.go`，新增）：
   - `TestCAPC6010HubCrashAroundPublish`：Hub 在 Publish activation 创建后崩溃，重启后同一 activation 完成且不产生重复 generation
   - `TestCAPC6013SQLiteBusyTransient`：SQLite busy/transient 错误恢复，验证 bounded retry 语义和无数据损坏
   - `TestCAPC6004EnhancedNoForwardAndUsageGeneration`：强化 CAP-C6-004，明确断言 428 forwarded=false、Adapter 无 request body、Usage 记录正确的 generation

6. **Security Suite**（`security_test.go`，15 场景）：
   - SEC-001：未认证 admin API 访问拒绝（401）
   - SEC-002：CSRF token 强制校验
   - SEC-003：Session cookie HttpOnly + Secure + SameSite=Strict
   - SEC-004：Snapshot 不泄漏 server-only 字段
   - SEC-005：Usage request detail 不含 prompt/body/secret
   - SEC-006：无效 enrollment code 拒绝
   - SEC-007：无效 access token Relay 拒绝且不 forward
   - SEC-008：严格 JSON body 校验（unknown fields/malformed）
   - SEC-009：Client header spoof 被 Relay 剥离
   - SEC-010：Secret value 创建后永不返回
   - SEC-011：Logout 失效 session
   - SEC-012：Client Control API 认证强制
   - SEC-013：Request body 大小限制
   - SEC-014：System status 不泄漏内部配置
   - SEC-015：幂等 Publish

7. **Resource Baseline**（`baseline_test.go`）：
   - 测量 Admin login/CRUD/Publish/convergence/runtime 延迟
   - 报告：`docs/s0-resource-baseline.md`

### Browser E2E 基础设施（新增）

1. **Playwright 配置**（`console/playwright.config.ts`）：
   - 使用 Chromium、单 worker、无 retry
   - 使用 production `dist/spa` + real Hub/Relay
   - 禁止 `page.route()` mock Admin API

2. **Browser E2E 测试**（`console/e2e/golden-path.spec.ts`）：
   - CAP-C6-001 browser slice：login → Overview → System → refresh
   - CAP-C6-001 browser slice：create user → user visible in list
   - **GAP**：当前 Browser E2E 只覆盖 Golden Path 的很小一段，缺完整 CAP-C6-001 流程

3. **@playwright/test 依赖**：
   - `console/package.json` 已添加 `@playwright/test@1.55.0`
   - `console/pnpm-lock.yaml` 已更新

### Makefile 测试入口分离（新增）

- `make system-test`：保持 bounded T3（默认 CI）
- `make s01-candidate-test`：完整 CAP-C6/C7 deterministic system scenarios（含 recovery/security）
- `make console-e2e`：Playwright browser T4.1 suite（需 production build + real Hub/Relay）
- `make console-build`：独立 Admin production build target

**GAP**：当前 `system-test` 和 `s01-candidate-test` 执行同一 `./test/system/...` 或 `./test/system/scenarios/` package，没有真正隔离 bounded T3 vs 完整 Candidate Gate。需要使用 build tags 或独立 smoke test 包隔离。

### .gitattributes 行尾规范化（新增）

- 新增 `.gitattributes` 确保 `*.go` 文件在所有平台 checkout 时保持 LF
- 解决 Windows `autocrlf=true` 导致 gofmt 全量报错的问题

### C7 Freeze Manifest 脚本增强

`scripts/freeze-manifest.mjs` 已增强，支持以下字段（在 C7 Gate 正式执行时生成）：
- `architectureCommit`：pinned architecture 仓库 commit
- `platformCoreCommit`：pinned platform-core commit
- `adminBuildHash`：Admin Console production build SHA-256
- `clientControlOpenApiHash` / `adminOpenApiHash`：OpenAPI SHA-256
- `canonicalFixtureHash`：canonical fixtures tree SHA-256
- `deterministicAdapterVersion`：Test Adapter + Test Client 源码 hash
- `realAdapterQualificationRef`：指向 `docs/s0-real-adapter-qualification.md`
- `resourceBaselineRef`：指向 `docs/s0-resource-baseline.md`
- `scenarioResults`：场景列表

**GAP**：
- `scenarioResults` 只是静态场景清单，没有真实 PASS/FAIL/result reference
- 没有 required `startedAt` / `completedAt`
- 缺 C0–C5 scenario results
- 可在 tests 未通过时直接运行
- Admin 未构建时允许 `adminBuildHash = "not-built"`
- `realAdapterQualificationRef` 只是指向一个尚未 qualification 的文档
- 没有证明 CAP-C7-002 的 clean replay

### 参考文档

- `docs/s0-real-adapter-qualification.md`：Real Adapter Qualification 报告（NOT EXECUTED）
- `docs/s0-resource-baseline.md`：Resource Baseline 基准指标（NOT GREEN — 多数指标未测量）
- `docs/s0-clean-replay-report.md`：Clean Replay 验证报告（NOT GREEN）

### 编译验证

```text
gofmt -l backend/           — PASS (零文件需格式化)
go build ./...              — PASS
go vet ./...                — PASS
go test -c ./test/system/... — PASS (编译通过)
pnpm build                  — PASS (production build)
pnpm typecheck              — PASS
tsc --noEmit -p tsconfig.e2e.json — PASS (Playwright E2E typecheck)
vitest run (13 files, 56 tests) — PASS
```

### C1–C3 前端产品闭环完成状态（2026-08-20 更新）

以下更新反映 2026-08-20 对 C1/C2/C3 前端产品需求的补齐：

### C1 Upstream Operational — ✅ Green
- ✅ Upstream candidate edit（Name、Base URL、Transport、Auth、Correlation、Usage level、Timeouts）
- ✅ Secret replace flow（create + replace，新 immutable version，active runtime 不变直到 Apply）
- ✅ Save / Discard / Reset 逻辑（save 后以服务端返回 revision 重建 baseline）
- ✅ 409 conflict handling（显示 currentConfigRevision，Discard 恢复到服务端最新）
- ✅ Candidate vs active revision 显示（pending 标识）
- ✅ Refresh recovery（刷新后恢复同一 activation）

### C2 Managed Resource Editor — ✅ Green
- ✅ Collection → editor/detail 模式（Model/TTS/ASR/MCP）
- ✅ 结构化 editor sections（Identity / Capability / Execution / Validation）
- ✅ Policy editor（Local coexistence toggles + Default Model/TTS/ASR pickers）
- ✅ Default pickers 仅列出 enabled 资源
- ✅ Relationship view（kind filter、click-through、missing binding 可操作错误、disabled/unverified/degraded 状态编码）
- ✅ Field-level validation navigation（error/warning 定位到资源）

### C3 Snapshot Projection & Preview — ✅ Green
- ✅ Review & Publish 结构化审查面（Added/Changed/Removed + Policy changes + Runtime routing impact + Warnings）
- ✅ Client Snapshot Preview（Providers/Models/TTS/ASR/MCP/Policy foldable detail）
- ✅ Client receives vs Client never receives 区分
- ✅ Publish progress 阶段显示（VALIDATING → STAGING_RELEASE → APPLYING_RUNTIME → FINALIZING → ACTIVE/FAILED）
- ✅ Recovery context（刷新后恢复 activation）

### Pricing — ✅ Green
- ✅ Resource/Upstream scope 字段
- ✅ KNOWN/PARTIAL/UNKNOWN cost 显示
- ✅ Cost completeness banner

### Usage — ✅ Green
- ✅ Semantic meters by kind（MODEL tokens、TTS chars/audio、ASR audio、MCP requests）
- ✅ UNKNOWN 不显示为 0
- ✅ Request detail 包含 cost & usage completeness

### Overview — ✅ Green
- ✅ Resource counts by kind
- ✅ Recent activation failures
- ✅ Usage/cost completeness status

### System — ✅ Green
- ✅ Spool state（usage ingest lag）
- ✅ Semantic orphan/unknown
- ✅ Control & reconciliation status（desired/applied revision + bundle hash + converged/pending）
- ✅ Latest activation detail（activationId + kind + state + errorCode + releaseId）
- ✅ Relay last seen

### 验证证据

```text
gofmt -l backend/           — PASS (零文件需格式化)
go build ./...              — PASS
go vet ./...                — PASS
pnpm build                  — PASS (production build)
pnpm typecheck              — PASS
tsc --noEmit -p tsconfig.e2e.json — PASS (Playwright E2E typecheck)
vitest run (13 files, 56 tests) — PASS
```

## 当前剩余缺口

1. **C6** — Not Green：Browser T4.1 Golden Path 未完成完整 CAP-C6-001 流程；一键 clean-environment T4.1 harness 不存在；CAP-C6-012 不是真实浏览器刷新；CAP-C6-013 不是真实 SQLite 锁竞争
2. **system-test vs s01-candidate-test 分层** — 假隔离：两者执行同一 scenarios package
3. **CAP scenario evidence mapping** — C0-C5 缺正式 CAP ID 映射
4. **Security required scenarios** — 缺 expired JWT/wrong audience/wrong issuer/unknown kid/disabled user/revoked device/revoked session/management endpoint/Set-Cookie/redirect/Secret in browser/Snapshot Preview leak
5. **Real Adapter qualification** — 未执行
6. **Freeze Manifest generator** — 不符合 C7（无真实 PASS/FAIL/startedAt/completedAt/C0-C5）
7. **Resource Baseline** — 多数指标未测量；文档事实错误已修正
8. **Clean Replay report** — 已修正为 NOT GREEN
9. **C7** — Not Started
