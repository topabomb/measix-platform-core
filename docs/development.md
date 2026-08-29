# Development Workflow

This document defines how engineers work on `measix-platform-core` locally or entirely through GitHub. It does not redefine platform behavior.

## 1. Toolchain baseline

S0 architecture fixes the major implementation stack:

- Go 1.26.x;
- Node.js >= 22.17.0;
- OpenAPI 3.0.3;
- `oapi-codegen/v2` / `kin-openapi`;
- SQLite with `modernc.org/sqlite`;
- Ent + Atlas versioned migrations;
- Vue 3 + TypeScript + Quasar;
- Pinia + Vue Router;
- `openapi-typescript`;
- pnpm 11.

Concrete minimum/runtime versions are enforced by repository-controlled toolchain files and CI. Do not duplicate floating version tables across multiple Markdown files.

## 2. Development modes

### Local-first

A local checkout is the fastest Red/Green loop:

```text
branch
→ smallest failing test
→ run narrow test locally
→ implement
→ rerun narrow test
→ affected component suite
→ push / PR
→ GitHub CI independently verifies T0–T3
```

Local configuration uses synthetic/test credentials only. Development must not require production configuration or public Provider access for normal T0–T3 work.

When a change affects an existing browser/system workflow, run the smallest relevant E2E slice explicitly where practical. Full T4.x remains a stage candidate/freeze gate rather than a per-commit loop.

`npm start` currently runs Hub, Relay and the Admin dev server through `concurrently`. It is a developer convenience only: it uses `go run`, omits Enterprise Tool Gateway and has no production restart/rate-limit/log-retention contract. Test harness process spawning is likewise test-only. Neither may be packaged or described as the S0.3 production supervisor.

### GitHub-only

When development is performed through GitHub/API/coding-agent access without a local executor:

```text
branch
→ Draft PR
→ commit Red test
→ GitHub Actions executes affected T0–T3 gate and fails as expected
→ inspect failing check/log
→ commit implementation
→ GitHub Actions executes latest SHA and passes
→ inspect checks/artifacts
→ refactor / rerun
```

GitHub Actions is the executor in this mode for the repository's normal T0–T3/build gates. T4.1 browser/system E2E is deliberately excluded from GitHub Actions CI/CD; C6/C7 completion requires a separate explicit candidate run on the exact candidate SHA.

Static code review alone is not test execution. See `docs/testing.md` (§11–18) for the TDD evidence contract.

## 3. I0 target repository structure

The implementation is organized around executable ownership, not around duplicating architecture documents:

```text
api/          executable wire contracts + canonical fixtures
backend/      Go binaries, packages, Ent, migrations
console/      Admin Console source/build/browser E2E
test/         qualification + S0 system harness
.github/      CI/PR automation
```

Subdirectories are created when their implementation lands. The source tree, not an old documentation snapshot, is authoritative for concrete package/file locations.

## 4. Bootstrap expectations

I0 must establish reproducible tool setup for both local CI-equivalent execution and GitHub Actions. Before I1 work begins, the repository must be able to:

- build the currently implemented `control-hub` and `runtime-relay` binaries; S0.3 later adds `enterprise-tool-gateway` as a real production binary;
- validate all four OpenAPI documents;
- reproduce generated Go/TS/Android wire artifacts or verify their exported generation inputs;
- replay SQLite migrations from an empty database;
- build the Quasar production shell;
- execute deterministic T0/T1/T2 CI.

The exact commands become part of repository tooling when those artifacts land. Documentation must be updated in the same PR that introduces or changes a command.

## 5. Branch/PR development

Use a short-lived branch for implementation work. Open a Draft PR early for multi-commit TDD and cross-component work.

The PR is the coordination object for:

- architecture linkage;
- Red/Green evidence;
- generated-code drift;
- migration review;
- required CI checks;
- review discussion;
- eventual release/test manifest references.

Direct pushes to `main` should stop once branch protection/required checks are enabled.

## 6. Code-generation workflow

For any OpenAPI/schema-derived artifact:

```text
change authoritative source
→ validate source
→ regenerate deterministically
→ inspect diff
→ run fixture/contract tests
→ run generated-drift check
→ commit source + expected generated artifacts together where repository policy requires them committed
```

Never make the generated output the first or only source of a protocol change.

## 7. Database workflow

Schema work follows:

```text
failing domain/repository/migration test
→ Ent schema change
→ Atlas migrate diff
→ review SQL
→ empty replay + upgrade test
→ implementation Green
```

See `docs/database-migrations.md`.

## 8. Frontend workflow

Admin Console development separates:

- generated Admin API types;
- API/problem/session infrastructure;
- Pinia workflow state;
- feature components;
- pages/layout;
- Playwright browser E2E under `console/e2e/`.

Frontend tests may stub Hub for component-level T1/T2, but system/browser lanes use the repository-wide real system harness. Browser E2E implementation rules live in `docs/admin-console-implementation.md`; architecture Admin/System Testing Specs remain the authority for required behavior and CAP scenarios.

Browser E2E is maintained as executable product evidence but is not part of the default GitHub Actions CI/CD path. During feature work, run affected slices explicitly when needed; before C6/C7/Freeze, run the complete T4.1 candidate suite on the pinned SHA.

## 9. Runtime Relay workflow

Relay data-path changes must be tested against real HTTP/TCP boundaries for streaming, cancellation, header handling and forwarding behavior. In-memory mocks do not replace required Relay T2/T3 scenarios.

## 10. System / E2E harness

S0.1 的 T3/T4.1 使用一套 repository-wide harness；Admin、Relay、Test Client 不各自建立一套端到端环境。Architecture Testing Specs 决定“必须证明什么”，本节只固定 core 中“怎样运行”。

### 10.1 Target structure

保持结构浅而清楚。Repository-wide harness 放在 Go module 内以便复用 `internal/` 包，落地为 `backend/test/system/`（路径结构沿用 `test/system/`，source-tree 权威见 §3）：

```text
backend/test/system/
├── harness/       environment/process/readiness/polling/log/cleanup
├── adapter/       deterministic upstream adapter
├── client/        client-facing Test Client
└── scenarios/     cross-component/system scenarios
```

执行入口：`make system-test`（等价 `cd backend && go test ./test/system/...`）。`ci-gate` 通过独立的 `system-test` job 承载当前 bounded T3 门禁；它不是 T4.1/browser E2E。

只有目录内文件数量或职责真正分化后才继续拆子目录；不要为每个 CAP ID、capability 或 process 创建一层目录。

职责边界：

- `harness/`：创建/销毁环境，不包含产品断言；
- `adapter/`：模拟外部 Upstream/Provider 边界并记录可安全断言的 transport fact；
- `client/`：只通过 client-facing Discovery/Session/Snapshot/Runtime contract 行为工作，不知道 `upstreamId`、`runtimeRouteId`、内部 URL 或 Secret；
- `scenarios/`：组合真实进程和角色并断言 architecture CAP 行为；
- `console/e2e/`：只负责真实浏览器中的 Admin 操作，不复制 system process orchestration。

### 10.2 Clean environment lifecycle

每个可独立执行的 system run 使用自己的临时根目录并拥有：

```text
temp root
├── SQLite database
├── generated test keys/config
├── process logs
└── browser/system artifacts

unique ports
real migrations
real Control Hub process
real Runtime Relay process
Deterministic Adapter
production Admin dist/spa when browser scenario needs it
Test Client when client-facing scenario needs it
```

Harness 生命周期固定为：

```text
allocate isolated environment
→ apply real migrations
→ create synthetic bootstrap identity/keys
→ start required real processes
→ bounded readiness polling
→ execute scenario
→ collect safe diagnostics on failure
→ terminate process group
→ remove/retain temp artifacts according to runner policy
```

要求：

- 默认不访问公网，不依赖 OpenAI/Google/其他真实 Provider；
- 使用动态/隔离 port，不假定开发机固定 8080/8090 可用；
- Hub/Relay 必须是本次待测构建的真实 process/binary；
- SQLite 使用真实 schema/migrations，不以内存 repository 替代；
- async convergence 使用有 deadline 的 polling/assertion，不使用大段固定 `sleep`；
- teardown 必须清理整个子进程组，测试失败也不能遗留 daemon；
- critical scenario 不通过 test retry 掩盖 race/flaky；
- logs/artifacts 不记录 Secret、Authorization、credential header、prompt/body。

Bootstrap 可以创建运行测试所必需的初始管理员/密钥，但 **被场景验证的业务对象必须通过该场景要求的公开产品边界创建**。例如 Admin Golden Path 不允许预写 Upstream/Resource/Release 到 DB 来绕过 UI。

### 10.3 Deterministic Adapter

Deterministic Adapter 是唯一有意替代外部服务的系统边界。它不是 mock Hub/Relay，而是一个真实监听 HTTP 的测试服务，能够确定性提供：

```text
normal request/response
SSE/stream chunks
binary TTS bytes
multipart ASR capture
MCP Streamable HTTP flow
4xx/5xx
timeout/slow response
cancellation observation
synthetic usage metadata
```

它可以记录用于断言的 transport fact，例如 method、path、允许的 headers、body hash、multipart field/part、stream/cancel observation；不得把 Secret/credential/plain prompt 直接写入持久测试报告。

### 10.4 Test Client

Test Client 验证 Android 之前的 server-side client-facing contract，不模拟 Android 内部实现。它只能从公开 client-facing topology 获得：

```text
Discovery / Session
Managed State / Snapshot
resourceId / runtimePath / generation
```

然后调用 Model streaming、TTS binary、ASR multipart、MCP Streamable HTTP 等 S0.1 required profile。测试代码不得从 Hub DB 或 Admin DTO 偷取 route/base URL/Secret 来完成 runtime 请求。

### 10.5 Browser integration

Playwright 浏览器由 `console/e2e/` 拥有，但环境由本 harness 启动。Browser lane 必须访问 production `dist/spa` 和真实 same-origin Admin API；禁止把 `page.route('/api/**')` mock response 当作 T4.1 Green 证据。

当前阶段先落一个最小 real-system smoke：

```text
clean environment
→ Hub/Relay ready
→ serve production SPA
→ browser login
→ Overview loads
→ System shows authoritative Hub/Relay state
→ refresh/navigation remain valid
```

之后随 C1–C5 增量增加 browser/system slice，C6 只把已经分别 Green 的能力组合成 clean-environment Golden Path 与恢复场景，而不是届时才首次建设 E2E。

这些 browser/system slices 可以在开发机或受控候选环境中显式执行，但不进入默认 GitHub Actions CI/CD。维护 E2E 与每次 push 自动执行 E2E 是两件不同的事。

### 10.6 CI layering and E2E execution policy

Repository verification 明确分成两条执行路径：

```text
GitHub Actions CI/CD
  T0      static / contract / generated drift
  T1/T2   backend unit/component + frontend unit/component/build
  T3      bounded cross-component system integration
  no T4.1 browser E2E

Explicit S0.1 candidate verification
  T4.1    production browser + Hub + Relay + Adapter + Test Client
  recovery/security/generation Golden Path
  real Adapter qualification as a separate explicit lane
```

GitHub Actions `ci-gate` 只聚合正常 T0–T3 required checks。它必须快速、确定、每个最新 PR SHA 都能可靠运行；不得因为没有执行 T4.1 就把 `ci-gate` 描述为 S0.1 complete。

完整 browser/system E2E 不运行在 GitHub Actions CI/CD 中。它在以下时点显式执行：

- C6 开始组合完整 Golden Path 时；
- 修复会影响现有 T4.1 主链/恢复语义的缺陷后；
- 生成 C7/Freeze candidate 前；
- candidate SHA 变化后重新做最终 candidate verification。

候选 E2E 必须固定 exact commit/build，使用生产 `dist/spa`、真实 Hub/Relay/SQLite/migrations、Deterministic Adapter/Test Client，并生成可追溯的 safe evidence。没有该证据，即使 GitHub Actions 全 Green，也不得宣称 C6/C7/S0.1 Freeze Green。

可以在 Actions 中保留快速 `make system-test`/system smoke，但它必须是 bounded T3，不启动完整 Playwright Golden Path。完整 E2E 的具体执行命令只有在实现真正落地时才加入 Makefile/package scripts；不要为了文档先发明不存在的命令。

### 10.7 TDD placement

E2E 不取代最窄层 Red → Green：

```text
field/payload/state bug
→ Unit/Component/Contract Red
→ implementation Green
→ affected system/browser slice explicit Green when relevant

cross-process/recovery bug
→ smallest reproducible T2/T3 scenario Red
→ implementation Green
→ related CAP/T4.1 explicit candidate scenario Green
```

关键 workflow 在其 C1–C5 实现阶段就维护对应 real-system/browser slice；不要等 C6 才补一整套事后验收测试。它们不必在每次 GitHub Actions run 自动执行。

## 11. Local runtime and dev commands

The repository root is npm-orchestrated (`package.json`); the Admin Console itself uses pnpm (`console/pnpm-lock.yaml`). Required local toolchain: Go 1.26.x, Node.js >= 22.17.0, pnpm 11. CI runs Node.js 22.17.0 to verify the minimum supported Node baseline.

```bash
npm run setup             # one-shot bootstrap: keys (.secrets/), migration (.data/hub.db), admin user
npm start                 # start control-hub + runtime-relay + Quasar dev server (concurrently)
npm run start:hub         # control-hub only      (http://localhost:8080)
npm run start:relay       # runtime-relay only    (:8090 public / 127.0.0.1:8091 internal)
npm run start:console     # Quasar dev server only (http://localhost:9000/admin/, proxies /api -> hub)
```

Notes:

- `npm run setup` is idempotent; generated secrets/passwords live under gitignored `.secrets/`, local databases under `.data/`.
- The Quasar dev server proxies `/api`, `/live`, `/ready`, `/.well-known` to the Hub (port 8080) and `/runtime` to the Relay (8090). Dev proxy is dev-only; production serves the SPA from the same origin as the Hub.
- `backend/cmd/devmigrate` applies the published Atlas migration SQL verbatim for local development only because the Atlas CLI cannot currently be installed alongside the local Go 1.26 toolchain. CI keeps executing the real `atlas migrate apply` replay (`make migration-replay`); `devmigrate` is not a replacement and never runs in CI.
- Other orchestration scripts: `npm run test` (backend + console), `npm run contract`, `npm run generate`, `npm run drift`, `npm run build`, `npm run ci` (delegates to the Makefile targets that GitHub Actions CI executes). These commands do not imply T4.1 browser E2E.
- System harness: `make system-test` runs `backend/test/system/` as the current bounded T3 lane. `make freeze-manifest` regenerates `docs/s0-freeze-manifest.json`; it is valid C7 evidence only after the explicit S0.1 T4.1 candidate gate and real Adapter qualification are Green for the pinned candidate.

## 12. Keeping docs accurate

When implementation changes how developers actually build/run/test/operate the repository, update the owning implementation document in the same PR. When behavior meaning changes, update architecture first instead.
