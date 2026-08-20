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
→ GitHub CI independently verifies
```

Local configuration uses synthetic/test credentials only. Development must not require production configuration or public Provider access for normal T0–T3 work.

### GitHub-only

When development is performed through GitHub/API/coding-agent access without a local executor:

```text
branch
→ Draft PR
→ commit Red test
→ GitHub Actions executes and fails as expected
→ inspect failing check/log
→ commit implementation
→ GitHub Actions executes latest SHA and passes
→ inspect checks/artifacts
→ refactor / rerun
```

GitHub Actions is the executor in this mode. Static code review alone is not test execution.

See `docs/tdd.md` for the evidence contract.

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

- build `control-hub` and `runtime-relay` health skeletons;
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

## 9. Runtime Relay workflow

Relay data-path changes must be tested against real HTTP/TCP boundaries for streaming, cancellation, header handling and forwarding behavior. In-memory mocks do not replace required Relay T2/T3 scenarios.

## 10. System / E2E harness

S0.1 的 T3/T4.1 使用一套 repository-wide harness；Admin、Relay、Test Client 不各自建立一套端到端环境。Architecture Testing Specs 决定“必须证明什么”，本节只固定 core 中“怎样运行”。

### 10.1 Target structure

保持结构浅而清楚：

```text
test/system/
├── harness/       environment/process/readiness/polling/log/cleanup
├── adapter/       deterministic upstream adapter
├── client/        client-facing Test Client
└── scenarios/     cross-component/system scenarios
```

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

### 10.6 CI layering

Repository CI 保持分层，不把所有测试塞入一个 job：

```text
T0      static / contract / generated drift
T1/T2   backend unit/component + frontend unit/component/build
T3      cross-component system integration
T4.1    production browser + Hub + Relay + Adapter + Test Client product/system gate
```

当 system harness 落地后，至少提供：

- 一个快速 `system-smoke`，用于持续验证 clean start、migration、Hub/Relay、production SPA 基础闭环；
- 一个完整 `system-e2e`/T4.1 gate，承载 architecture required CAP-C6/failure/recovery 场景。

具体命令名以 Makefile/package scripts/CI workflow 为实现权威，新增命令时同步本文。S0.1 checkpoint/Freeze completion 必须引用最新 commit 上实际执行的 Green evidence；历史 Green、component mock Green 或人工页面检查不能替代 T4.1。

### 10.7 TDD placement

E2E 不取代最窄层 Red → Green：

```text
field/payload/state bug
→ Unit/Component/Contract Red
→ implementation Green
→ affected system/browser slice Green

cross-process/recovery bug
→ smallest reproducible T2/T3 scenario Red
→ implementation Green
→ related CAP/T4.1 scenario Green
```

关键 workflow 在其 C1–C5 实现阶段就加入对应 real-system slice；不要等 C6 才补一整套事后验收测试。

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
- Other orchestration scripts: `npm run test` (backend + console), `npm run contract`, `npm run generate`, `npm run drift`, `npm run build`, `npm run ci` (delegates to the Makefile targets that CI executes).

## 12. Keeping docs accurate

When implementation changes how developers actually build/run/test/operate the repository, update the owning implementation document in the same PR. When behavior meaning changes, update architecture first instead.
