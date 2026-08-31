# Development Workflow

This document owns executable local workflows, not platform semantics. Toolchain versions come from `backend/go.mod`, package manifests/lockfiles and `.github/workflows/ci-gate.yml`; architecture does not pin patch versions.

## 1. Environment and source layout

Use the repository-pinned Go/Node/pnpm toolchains. Atlas and GNU Make with a POSIX shell are required for the complete Make/CI workflow. Native PowerShell can run the direct Go/Node/pnpm commands below; it is not a POSIX Make recipe executor.

Current layout:

```text
api/                   four OpenAPI documents, fixtures, Android export
backend/cmd/           Hub, Relay, development/export utilities
backend/internal/      common, generated wire, Hub and Relay implementation
backend/ent/           schema and generated persistence code
backend/migrations/    versioned SQL and Atlas checksum
backend/test/system/   Go harness, deterministic adapter/client, tagged scenarios
console/src/           Admin UI
console/e2e/           browser assertions
scripts/               Node browser/candidate orchestration and evidence tooling
docs/                  implementation instructions and evidence
```

Gateway source/OpenAPI and production service packaging are S0.3 work, not current directories.

## 2. Local bootstrap and startup

Install dependencies from the root and console lockfiles. `npm run setup` invokes `scripts/dev-setup.mjs`: exclusively creates missing synthetic key files, applies strict development migrations and bootstraps with `--if-empty`. A repeat against a current managed development DB preserves keys/credentials. It reports the protected password-file location, not plaintext. This is not a production installer or reset tool; legacy development migration ledgers fail closed for manual review.

`npm start`/`npm run dev` starts the development Hub, Relay and console; usage ingestion targets private Hub port 8081. Alternatively, run these in separate terminals **from backend/** using setup's synthetic files:

```text
go run ./cmd/control-hub run --listen 127.0.0.1:8080 --internal-listen 127.0.0.1:8081 --db ../.data/hub.db --master-key-file ../.secrets/master.key --jwt-private-key-file ../.secrets/jwt-ed25519.seed --relay-internal-url http://127.0.0.1:8091 --relay-service-token-file ../.secrets/relay-service.token

go run ./cmd/runtime-relay --public-listen 127.0.0.1:8090 --internal-listen 127.0.0.1:8091 --spool ../.data/relay-spool.db --hub-usage-url http://127.0.0.1:8081/internal/v1/usage/request-events:batch --hub-service-token-file ../.secrets/relay-service.token
```

In another terminal from the repository root:

```text
pnpm -C console dev
```

These are development HTTP endpoints, not production origin/TLS qualification. The console dev server/proxy is not a packaged production ingress. To exercise Hub static hosting, first build the console then add `--admin-assets-dir ../console/dist/spa` to Hub; same-origin runtime routing still needs the ingress/proxy. `go run`/`concurrently` provide no production restart/rate-limit/log-retention guarantee.

## 3. Normal checks

From `backend/`:

```text
go test ./... -count=1
go vet ./...
go test ./internal/contract -count=1
go test -tags=smoke ./test/system/scenarios/ -count=1 -timeout 5m
go test ./test/system/adapter/ ./test/system/client/ -count=1 -timeout 2m
```

From the repository root:

```text
pnpm -C console typecheck
pnpm -C console test --run
pnpm -C console build
```

Ordinary `go test ./...` does not execute build-tagged smoke/candidate scenarios. Build, unit and component tests do not prove browser, real Adapter, Android or Freeze acceptance.

From either PowerShell or POSIX, `node scripts/checks.mjs generate` owns regeneration; `fmt`, `drift` and `static` are sibling commands. `node --test scripts/checks.test.mjs scripts/freeze-manifest.test.mjs` validates failure/pin rules. In POSIX, `make ci` regenerates first and serializes prerequisites; `generated-drift` alone remains only a diff check. Generation intentionally can change derived files: inspect and commit source plus expected outputs together, never hand-edit generated types.

## 4. API and database changes

Semantic changes start in the owning architecture contract, then OpenAPI → canonical fixtures → generated artifacts → tests → implementation. `make generate` delegates to that same Node owner and installs locked console dependencies before generation. It covers four Go wire surfaces, Android Client OpenAPI export/manifest, Ent, Admin TypeScript and migration checksum. Android export is not Kotlin consumer implementation. See [API contracts](api-contracts.md).

Schema changes require reviewed versioned SQL, checksum, empty replay and an upgrade fixture preserving existing facts. `devmigrate` is a development convenience with different revision bookkeeping from Atlas; it is not an equivalent release migration gate. See [database migrations](database-migrations.md).

## 5. System and browser ownership

There are two real implementations of test orchestration, not one physical harness:

- `backend/test/system/{harness,adapter,client,scenarios}`: Go component/system environment and tagged scenarios.
- `scripts/lib/harness.mjs`, `scripts/e2e-harness.mjs`, `scripts/candidate-orchestrator.mjs`: Node process/static-host/browser/candidate orchestration.
- `console/e2e/`: browser actions/assertions; it must not recreate its own daemon lifecycle.

Keep orchestration out of feature tests. Share contracts/fixtures and align evidence, rather than declaring the two environments identical. A scenario requiring browser → traffic → Usage/System closure must run those steps against the **same** runtime, not combine unrelated Green runs.

Bounded T3 is `make system-test`; the explicit S0.1 candidate lanes are `make s01-candidate-test` and `make s01-browser-candidate`. The browser entry builds production SPA and runs `node scripts/e2e-harness.mjs`; run it on an isolated candidate because it creates processes and artifacts. See [Playwright notes](playwright-e2e-notes.md).

Harness requirements: isolated DB/ports, real migrations and real component processes, synthetic secrets, deadline polling, reliable teardown, safe diagnostics. Bootstrap may create initial identity/keys; business objects under test must use the declared public/Admin product surface, not direct DB writes or Relay internal control shortcuts.

## 6. TDD, CI and evidence

Use a meaningful observed Red → Green → Refactor loop for behavior/regressions; documentation-only changes do not require artificial Red. Run the narrow test, affected component checks and real-boundary tests appropriate to risk.

GitHub-only work uses a Draft PR and actual check/log inspection; current CI triggers on PRs to `main` and pushes to `main`, not arbitrary branch pushes. CI's four work jobs are static-contract, backend-test, system-test and console-test, aggregated by ci-gate. It excludes browser T4.1 and real external qualification.

Evidence tooling rejects failed commands, dirty/mismatched source/build/contract/artifact pins and incomplete one-run Adapter profiles. It never overwrites the historical manifest. The CAP runner is explicitly S0.1-only and rejects current Snapshot v2; independent clean-source replay is not implemented (runtime-only diagnostics cannot finalize C7). Therefore `make freeze-gate` is not a working S0.2 release path. See [testing](testing.md) and [release](release.md); never infer acceptance from script names or a historical manifest.
