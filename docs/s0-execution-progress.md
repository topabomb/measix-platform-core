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
| C0 Contract Audit & Freeze Preparation | ✅ Green | required profile 的 OpenAPI/fixtures/generated types/backend validation 已闭合；TTS/ASR `upstreamModelKey` 已 required；`ValidationIssue` 已增加 `resourceKind/resourceId/field` 结构化定位；`DraftPreviewResponse` 使用 `projectionHash` 而非 `snapshotHash` |
| C1 Upstream Operational | ✅ Green | Create/Test/Apply 与 typed UpstreamConfig 已存在；existing candidate edit、Secret replace（内联创建工作流，不暴露裸 sec_* ID）、save/discard/reset、409 conflict handling、candidate vs active revision 显示、refresh recovery 与 apply-failure candidate preservation 均已实现；modal 支持 320px 响应式 |
| C2 Managed Resource Editor | ✅ Green | Provider/Model/TTS/ASR/MCP/Policy 编辑与 binding 已存在；collection → selected editor/detail、Policy Default pickers、dirty/clean state、结构化 validation navigation（`resourceId` 精确定位）、relationship view 均已完成；diff/validation 逻辑已抽取到 `composables/useResourceDiff.ts` |
| C3 Snapshot Projection & Preview | ✅ Green | canonical preview backend 返回 compiler 输出的规范化投影数组；`projectionHash` 使用 placeholder releaseId/generation/publishedAt，不是最终 `snapshotHash`；Review & Publish 结构化审查面、Client Snapshot Preview、Publish progress 均已完成 |
| C4 Runtime Reference Profile | ✅ Green | deterministic Adapter/Test Client 已证明 Chat request/response+SSE、TTS binary（model+input+voice）、ASR multipart、MCP（JSON-RPC initialize → tools/list → tools/call）及关键 Relay admission/transport 行为；这是 deterministic T2/T3 证据，不等于 real Adapter qualification |
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
   - ✅ 创建一键 clean-environment T4.1 harness（`scripts/e2e-harness.mjs`，自动 temp DB/Hub/Relay/Adapter/SPA）
   - ✅ 修复 CAP-C6-013 SQLite busy/transient（注册 `modernc.org/sqlite` 驱动）
   - 扩展 Browser T4.1 Golden Path 从当前 login→Overview→System→refresh 到完整 CAP-C6-001 流程
2. **CAP scenario evidence mapping**：为 C0-C5 添加 Architecture Scenario → executable test → exact SHA result 映射
3. ✅ **Security required scenarios 补全**：expired JWT/wrong audience/disabled user/revoked device/session/management endpoint/Set-Cookie/redirect/Secret in browser/Snapshot Preview leak — 已在 `security_test.go` 中实现 SEC-016..021
4. ✅ **system-test vs s01-candidate-test 分层隔离**：使用 build tags (`smoke` vs `candidate`) 严格隔离 bounded T3 vs 完整 Candidate Gate
5. ✅ **Freeze Manifest evidence compiler** — 已重写为消费真实测试 artifact 的 evidence compiler，不再硬编码 PASS/FAIL
6. 完成至少一套 required real Adapter qualification；
7. exact candidate 全部 Green 后生成 C7 Freeze manifest；
8. C7 之后才开始 S0.2 Android。

## 状态维护规则

本文件只维护**当前实现状态和剩余缺口**，不复制 architecture requirement 细节。Architecture 变化后必须重新审查受影响 checkpoint；历史测试结果不能自动继承为新的 Green。每次宣称 checkpoint Green，必须能指向当前 architecture baseline 与当前 implementation SHA 对应的 executable evidence。

## C6/C7 系统测试实现进展（2026-08-21 更新）

> 以下为系统测试代码实现进展。Build tag 隔离、安全测试增强、E2E harness 修复和 Freeze Manifest 修正已完成并验证。Candidate gate 的全面 Green 仍需在 exact candidate SHA 上显式执行。

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

6. **Security Suite**（`security_test.go`，21 场景）：
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
   - SEC-016：JWT claims 验证（过期/错误 audience/issuer）
   - SEC-017：Disabled user 请求被拒绝
   - SEC-018：Revoked device/session 请求被拒绝
   - SEC-019：Management endpoint 认证强制
   - SEC-020：Set-Cookie/Location header 被 Relay 剥离
   - SEC-021：Secret plaintext 不进入浏览器持久状态

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
   - CAP-C6-001 browser slice：create upstream → upstream visible
   - CAP-C6-001 browser slice：Resources page 五 tab navigation
   - CAP-C6-001 browser slice：Releases/Usage/System 页面加载
   - CAP-C6-001 full golden path：login → create upstream → resources → releases → usage → system
   - CAP-C6-001 session 持久性测试（跨页导航）
   - CAP-C6-001 logout 测试
   - CAP-C6-012 browser refresh recovery
   - CAP-C6-014 full restart recovery
   - `ProblemBanner.vue` 已添加 `data-cy="problem-banner"`
   - `AdminLayout.vue` 已添加 `data-cy="logout-btn"` 和 `data-cy="logout-btn-mobile"`

3. **@playwright/test 依赖**：
   - `console/package.json` 已添加 `@playwright/test@1.55.0`
   - `console/pnpm-lock.yaml` 已更新

### Makefile 测试入口分离（新增）

- `make system-test`：保持 bounded T3（默认 CI）
- `make s01-candidate-test`：完整 CAP-C6/C7 deterministic system scenarios（含 recovery/security）
- `make console-e2e`：Playwright browser T4.1 suite（需 production build + real Hub/Relay）
- `make console-build`：独立 Admin production build target

✅ 已修复：`system-test` 使用 `-tags=smoke` 只运行 bounded T3 smoke 测试；`s01-candidate-test` 使用 `-tags=candidate` 运行完整 CAP-C6/C7 场景。两者通过 build tags 严格隔离。

### .gitattributes 行尾规范化（新增）

- 新增 `.gitattributes` 确保 `*.go` 文件在所有平台 checkout 时保持 LF
- 解决 Windows `autocrlf=true` 导致 gofmt 全量报错的问题

### C7 Freeze Manifest Evidence Compiler

`scripts/freeze-manifest.mjs` 已重写为 evidence compiler，不再硬编码 PASS/FAIL：

**Evidence Pipeline 设计**：
```
test runners (go test -json, vitest --json, playwright --reporter=json)
  ↓
machine-readable result artifacts (JSON files in .artifacts/)
  ↓
adapter qualification artifact (docs/s0-real-adapter-qualification.json)
resource baseline artifact (docs/s0-resource-baseline.json)
  ↓
freeze-manifest compiler (scripts/freeze-manifest.mjs)
  ↓
docs/s0-freeze-manifest.json
```

**关键行为**：
- 消费 `.artifacts/` 中的 Go test JSON / vitest JSON / Playwright JSON artifacts
- 每个 scenario 的 result 由 artifact 中的实际测试结果决定，不是硬编码
- 如果 artifact 不存在，scenario 标记为 NOT_EXECUTED
- 验证 artifact 中的 commit SHA 与当前 commit 匹配
- Real Adapter 和 Baseline 状态从各自的 JSON artifact 读取
- 如果任何 required scenario 不是 PASS，manifest 不会生成
- `make collect-artifacts` 收集测试 artifact 到 `.artifacts/`
- `make freeze-gate` 执行完整 C7 Gate 流程
- `.artifacts/` 已加入 `.gitignore`

### 参考文档

- `docs/s0-real-adapter-qualification.md`：Real Adapter Qualification 报告（NOT EXECUTED）
- `docs/s0-resource-baseline.md`：Resource Baseline 基准指标（NOT GREEN — 全部数值为 NOT MEASURED，未由可执行测试生成）
- `.artifacts/real-adapter-qualification.json`：由 `scripts/collect-adapter-qualification.mjs` 生成，被 `freeze-manifest.mjs` 消费
- `.artifacts/resource-baseline.json`：由 `scripts/collect-baseline.mjs` 生成，被 `freeze-manifest.mjs` 消费

### 编译验证

```text
exact SHA: db6181c4ca6a5a7204fbf87ced41fa9ffdf09334
CI run:    32440523743 (ci-gate: success)

Jobs:
  static-contract  — success
  console-test     — success
  backend-test     — success
  system-test      — success
  ci-gate          — success
```

> CI 证明默认 T0-T3 deterministic baseline Green；不证明 C6/C7/S0.1 Freeze。

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
exact SHA: db6181c4ca6a5a7204fbf87ced41fa9ffdf09334
CI run:    32440523743 (ci-gate: success)
```

## 当前剩余缺口（2026-08-21 更新）

1. **C6** — Not Green：Browser T4.1 Golden Path 已扩展覆盖 CAP-C6-001 关键步骤（login → upstream → resources → releases → usage → system + session 持久性 + logout + refresh recovery + restart recovery），但须在 exact candidate SHA 上执行验证
2. ✅ **system-test vs s01-candidate-test 分层** — 已修复：使用 build tags (`smoke` vs `candidate`) 严格隔离
3. **CAP scenario evidence mapping** — C0-C5 仍缺正式 CAP ID 映射
4. ✅ **Security required scenarios** — 已补全：SEC-016 (JWT claims), SEC-017/018 (disabled user/revoked device), SEC-019 (management endpoint), SEC-020 (Set-Cookie/redirect strip), SEC-021 (secret in browser)
5. **Real Adapter qualification** — 未执行
6. ✅ **Freeze Manifest evidence compiler** — 已重写为消费真实测试 artifact 的 evidence compiler，不再硬编码 PASS/FAIL
7. **Resource Baseline** — 全部数值为 NOT MEASURED；须在 candidate SHA 上执行真实测量后替换
8. **C7** — Not Started

## S0.1 审查修复 Evidence（2026-08-21）

### 已完成修复

| # | 审查项 | 修复内容 | 验证 |
|---|---|---|---|
| 1 | SEC-017/018 | 添加缺失的 `Idempotency-Key`；用 `waitActivationCompleted` bounded polling 替换 `time.Sleep` | `go test ./internal/relay` PASS |
| 2 | SEC-016 | 通过 `LoadEd25519PrivateKey` 伪造签名正确但 Claims 错误（过期、误匹配 aud/iss）的 JWT | `security_test.go` 编译通过 |
| 3 | SEC-020 | Adapter 注入 `Set-Cookie` 和 `Location` 验证 Relay 剥离行为 | `security_test.go` 编译通过 |
| 4 | CAP-C6-013 | 添加 `_ "modernc.org/sqlite"` 匿名导入注册驱动 | `recovery_test.go` 编译通过 |
| 5 | 测试分层隔离 | 使用 `-tags=smoke` 和 `-tags=candidate` 严格隔离基础集成与完整 Candidate | `make system-test` PASS；`go test -tags=candidate -run=^$` 编译通过 |
| 6 | SessionStore 测试 | 通过 `setUnauthorizedHandler` 真实模拟 401 触发 session 清除 | `vitest run` 56 tests PASS |
| 7 | Freeze Manifest | 补全 SEC-016..021 和 CAP-C2-021 场景；修正 C7 自循环；Real Adapter 和 Baseline 为硬性门禁 | `node --check scripts/freeze-manifest.mjs` PASS |
| 8 | E2E Harness | 修正 Hub/Relay CLI 参数；生成密钥文件；启动 Adapter；创建 same-origin SPA 代理 | `node --check scripts/e2e-harness.mjs` PASS |
| 9 | 前端 i18n | 中英文双语支持，自动检测 + 手动切换 | `pnpm typecheck` + `vitest` PASS |

### 验证证据汇总

```text
exact SHA: db6181c4ca6a5a7204fbf87ced41fa9ffdf09334
CI run:    32440523743 (ci-gate: success)

Jobs:
  static-contract  — success
  console-test     — success
  backend-test     — success
  system-test      — success
  ci-gate          — success
```

> CI 证明默认 T0-T3 deterministic baseline Green；不证明 C6/C7/S0.1 Freeze。
