# Development Workflow

This document owns executable local workflows, not platform semantics. Toolchain versions come from `backend/go.mod`, package manifests/lockfiles and `.github/workflows/ci-gate.yml`; architecture does not pin patch versions.

## 1. Environment and source layout

Use the repository-pinned Go/Node/pnpm toolchains. GNU Make with a POSIX shell is required for the complete Make/CI workflow. Native PowerShell can run the direct Go/Node/pnpm commands below; it is not a POSIX Make recipe executor.

Current layout:

```text
api/                   four OpenAPI documents, fixtures, Android export
backend/cmd/           Hub, Relay, development/export utilities
backend/internal/      common, generated wire, Hub and Relay implementation
backend/ent/           schema and generated persistence code
backend/migrations/    ordered embedded Hub database migrations
backend/test/system/   Go harness, deterministic adapter/client, tagged scenarios
console/src/           Admin UI
console/e2e/           browser assertions
scripts/               Node browser/candidate orchestration and evidence tooling
docs/                  implementation instructions and evidence
```

Gateway source/OpenAPI and production service packaging are S0.3 work, not current directories.

## 2. Local bootstrap and startup

Install dependencies from the root and console lockfiles. `npm run setup` invokes `scripts/dev-setup.mjs`: exclusively creates missing synthetic key files, applies or verifies the shared migration history and bootstraps with `--if-empty`. A repeat against a current managed development DB preserves keys/credentials. It reports the protected password-file location, not plaintext. This is not a production installer or reset tool. It uses the shared append-only migration owner described in [Database migrations](database-migrations.md); unrecognized data or checksum conflicts stop startup without deleting business data.

`npm start`/`npm run dev` starts the development Hub, Relay and console; usage ingestion targets private Hub port 8081. Alternatively, run these in separate terminals **from backend/** using setup's synthetic files:

```text
go run ./cmd/control-hub run --listen 127.0.0.1:8080 --internal-listen 127.0.0.1:8081 --db ../.data/hub.db --master-key-file ../.secrets/master.key --jwt-private-key-file ../.secrets/jwt-ed25519.seed --relay-internal-url http://127.0.0.1:8091 --relay-service-token-file ../.secrets/relay-service.token

go run ./cmd/runtime-relay --public-listen 127.0.0.1:8090 --internal-listen 127.0.0.1:8091 --spool ../.data/relay-spool.db --hub-internal-url http://127.0.0.1:8081 --hub-service-token-file ../.secrets/relay-service.token
```

In another terminal from the repository root:

```text
pnpm -C console dev
```

These are development HTTP endpoints, not production origin/TLS qualification. To exercise the complete same-origin path, build the console and Portal, configure their Hub asset directories, then use the checked-in [Caddy ingress recipe](operations.md#one-public-origin). Discovery, enrollment, Snapshot, Runtime and Portal must use this public origin. `go run`/`concurrently` provide no production restart/rate-limit/log-retention guarantee.

### Actual Android / Admin development environment

Run `npm run device:real` from the Core root. It builds production Admin/Portal assets, runs shared database migrations, preserves deployment credentials, starts the local same-origin Hub/Relay, and publishes the explicit v5 preset. The actual origin is printed after readiness and stored in `.data/device-real/process.json`; do not reuse an old LAN address. The Admin password is in ignored `.secrets/device-real-admin-password.txt`.

Every invocation builds the current local Core and sibling Portal working trees, including uncommitted source changes. It stops the previously owned process and runs the newly built binary with a unique `buildVersion`, recorded in `process.json` and the publisher result. Before changing any preset configuration, the publisher authenticates and checks that both Hub and Relay report this invocation's build identity through Admin System Status. A responding older process at the public origin is rejected; a Relay still starting has a bounded wait to report its identity. Readiness alone does not establish which build is serving the origin.

The preset owns the isolated `.data/device-real` draft: rerunning restores its predefined resources and three complete Starter openings. Do not point it at a shared or production database. Manual Admin publications remain immutable releases, but their edits are not the preset's next draft. `npm run device:real:stop` stops its owned process. `device:real:reset` deletes isolated data and is only for an explicit decision to discard it, never an upgrade/checksum repair.

When replacing the preset draft, echo the existing server-owned `toolDiscovery` unchanged for the matching MCP ID. Discovering tools in Admin must not break a later restart. ALL remains explicit, with an empty `allowedTools` list; it does not require a new discovery. An unchanged client projection keeps its current published generation while the program and static assets are rebuilt. A changed projection publishes and reports the generation from the completed activation.

Supplier credentials come from ignored `.secrets/supplier-keys.env`; existing ACTIVE upstreams and their Secret references are reused. Changing the file does not rotate a saved Secret: use normal Admin Secret/upstream candidate/apply actions for intentional rotation. ACTIVE and `/ready` prove configuration activation and process readiness, not supplier authorization or model availability. Verify actual resource invocation separately and retain the provider diagnostic on failure. Missing Starter opening, validation or migration errors stop publication/startup with their original code and path.

Publisher results are saved in `.data/device-real/logs/preset-result.json`. The launcher clears the old result before each invocation and includes the current endpoint, HTTP status and problem code (or publisher exit code if no result was written) in its final failure. API response bodies and credentials are excluded. Preserve the database, SQLite sidecars, protected files and diagnostic on failure; rerun after correcting the reported cause.

Use actual Admin authoring and a dedicated Android emulator for UI/context verification. Keep the retained production demo separate. [Starter verification](starter-opening-snapshots.md#10-实施及验收记录) records browser, device, deterministic-adapter and supplier boundaries separately.

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

From either PowerShell or POSIX, `node scripts/checks.mjs generate` owns regeneration; `fmt`, `drift` and `static` are sibling commands. `npm run test:tooling` validates failure/pin rules, the command-line contract of every `./cmd` invocation in repository tooling, and that each required freeze artifact has a producer that also writes its metadata. These are local tools: `make ci` runs tests only and does not regenerate or compare generated files, so run `make generate` yourself after changing contracts or fixtures and commit the derived output with the source. Generation intentionally can change derived files: inspect and commit source plus expected outputs together, never hand-edit generated types.

## 4. API and database changes

Semantic changes start in the owning architecture contract, then OpenAPI → canonical fixtures → generated artifacts → tests → implementation. `make generate` delegates to that same Node owner and installs locked console dependencies before generation. It covers four Go wire surfaces, Android Client OpenAPI export/manifest, Ent, canonical client fixtures, the Android integration export and Admin TypeScript. It does not produce a schema checksum file: there is none. Android export is not Kotlin consumer implementation. See [API contracts](api-contracts.md).

Schema changes add an immutable, sequential SQL migration and update Ent/generated code. Preserve a previous-version fixture when a new migration is introduced, and test empty initialization, upgrade data preservation, idempotence, per-file atomic failure and backup/recovery. `devmigrate` is only a compatibility wrapper around the same embedded migrator used by `control-hub migrate`. See [database migrations](database-migrations.md).

## 5. System and browser ownership

There are two real implementations of test orchestration, not one physical harness:

- `backend/test/system/{harness,adapter,client,scenarios}`: Go component/system environment and tagged scenarios.
- `scripts/lib/harness.mjs`, `scripts/e2e-harness.mjs`: Node process/static-host/browser/candidate orchestration. The browser candidate gate has a single entry (`node scripts/e2e-harness.mjs`); no parallel orchestrator or artifact exists.
- `console/e2e/`: browser actions/assertions; it must not recreate its own daemon lifecycle.

Keep orchestration out of feature tests. Share contracts/fixtures and align evidence, rather than declaring the two environments identical. A scenario requiring browser → traffic → Usage/System closure must run those steps against the **same** runtime, not combine unrelated Green runs.

Bounded T3 is `make system-test`; the explicit S0.1 candidate lanes are `make s01-candidate-test` and `make s01-browser-candidate`. The browser entry builds production SPA and runs `node scripts/e2e-harness.mjs`; run it on an isolated candidate because it creates processes and artifacts. Browser entrypoints and failure diagnosis are in [testing](testing.md).

Harness requirements: isolated DB/ports, real migrations and real component processes, synthetic secrets, deadline polling, reliable teardown, safe diagnostics. Bootstrap may create initial identity/keys; business objects under test must use the declared public/Admin product surface, not direct DB writes or Relay internal control shortcuts.

## 6. TDD, CI and evidence

Use a meaningful observed Red → Green → Refactor loop for behavior/regressions; documentation-only changes do not require artificial Red. Run the narrow test, affected component checks and real-boundary tests appropriate to risk.

GitHub-only work uses a Draft PR and actual check/log inspection; current CI triggers on PRs to `main` and pushes to `main`, not arbitrary branch pushes. CI's four work jobs are static-contract, backend-test, system-test and console-test, aggregated by ci-gate. It excludes browser T4.1 and real external qualification.

Evidence tooling rejects failed commands, dirty/mismatched source/build/contract/artifact pins and incomplete one-run Adapter profiles. It creates new candidate artifacts without overwriting existing files. The CAP runner pins current v5 resource/contract evidence, including shared v4/v5 defaults and strict Starter opening wire checks; it does not replace Starter product/consumer verification or S0.2 ERX; independent clean-source replay rebuilds pinned commits and reruns the required path before finalization. See [testing](testing.md) and [release](release.md); candidate acceptance requires each named gate rather than a wrapper target.
