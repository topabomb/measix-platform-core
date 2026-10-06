# Testing, CI and TDD

This document defines how `measix-platform-core` executes and records tests, and the TDD discipline that governs them. Required behavior and critical scenarios remain authoritative in the S0.1/S0.2/S0.3/S0.4/Component/System Testing Specs in `topabomb/measix-architecture`.

## 1. Test layers

| Layer | Purpose | Repository implementation |
|---|---|---|
| T0 Static / Contract | current schema, codegen, fixture, build consistency | contract tests, evidence/tooling regression tests, console typecheck and production build. Regeneration/drift and schema replay are freeze-time collectors, not per-commit CI |
| T1 Unit / Domain | pure validation/state/mapping | Go unit tests, Vitest unit/store helpers |
| T2 Component Integration | one real component + local real boundaries | real SQLite, real HTTP server, component/static-host tests |
| T3 Cross-component Integration | multiple real MEASIX components | real Hub↔Relay, Admin↔Hub where implemented, deterministic Adapter |
| T4.1 S0.1 Product/System E2E | pre-Android product topology | production browser + real Hub/Relay + Adapter + Test Client |
| T4.2 S0.2 Realm/Experience Product | real Realm/Portal/product projection | pinned server + Android/Portal evidence as required |
| T4.3 S0.3 Gateway Product | three-daemon server product | production Admin + real Hub/Gateway/Relay + downstream MCP + Test Client |
| T4.4 S0.4 Android Integration | full managed Android profile | real emulator/device + pinned Hub/Gateway/Relay |
| T4 Final S0 System / RC | frozen S0.1–S0.4 composition | cross-repository final system RC |

Normal GitHub Actions CI is deliberately limited to deterministic T0–T3 plus required builds. Browser/System E2E is a promotion/freeze proof rather than a per-commit feedback loop. S0.1 still requires the complete T4.1 gate before C6/C7 completion.

## 2. Delivery-gate mapping

### S0 Core

Existing I0–I5 tests remain regression evidence for identity, Draft/Snapshot foundations, Relay admission/transport, metering, persistence and Admin infrastructure. They do **not** by themselves prove S0.1 or final S0 Exit.

### S0.1 Managed Capability Delivery

S0.1 is a pre-Android product/system gate owned semantically by `measix-s0-capability-delivery-system-testing-spec.md`.

```text
real Admin browser
  → real Control Hub
  → real Runtime Relay
  → deterministic Test Adapter / qualified real Adapter

Snapshot/Runtime Test Client
  → real Client Control API + Runtime API
```

It must prove the required `CAP-*` scenarios, including Managed Capability profiles, Snapshot preview/release equivalence, publish/runtime enforcement, usage/pricing/diagnostics, no-forward security behavior and Client Contract Freeze evidence.

### S0.2–S0.4 / final S0 Exit

S0.2 consumes the pinned S0.1 freeze for Realm/Experience. S0.3 adds the Enterprise Tool Gateway server/Admin/Test Client closure and production supervision/logging proof. S0.4 adds real `rikkahub_mcp` Android full-profile integration. Final S0 RC proves applicable `ERX-*`/`ETG-*`/`AND-*`/`SYS-*` scenarios with fixed cross-repository commits. An earlier-stage Green must never be reported as a later Freeze or final S0 Exit.

## 3. Current test locations

Concrete current paths are:

```text
backend/**/*_test.go             Go tests (some system scenarios require build tags)
console/src/**/*.test.ts         Vitest unit/component tests
api/fixtures/                   canonical contract fixtures
backend/test/system/            current Go-module deterministic/system harness
console/e2e/*.spec.ts            existing Playwright browser tests
scripts/                        Node browser/candidate/replay/qualification orchestration
```

Within `backend/test/system/`:

```text
harness/       process/environment/readiness/cleanup
adapter/       deterministic upstream adapter
client/        client-facing Test Client
scenarios/     cross-component/system scenarios
```

Architecture documents may describe the logical repository-wide `test/system` responsibility. The current physical Go implementation is `backend/test/system/`; source tree is the concrete implementation truth. Do not document an unimplemented directory as if it already exists.

Adapter qualification/report locations may evolve as executable harness work lands; the source tree and release tooling are authoritative for the physical path, while architecture Qualification Spec remains authoritative for what evidence is required.

## 4. Mapping architecture requirements

Critical architecture scenarios use stable IDs such as `HUB-*`, `RLY-*`, `ADM-*`, `CAP-*`, `ERX-*`, `ETG-*`, `AND-*` and `SYS-*`.

- a test proving a critical scenario exposes the ID in its name, metadata or nearby comment;
- ordinary unit tests do not need artificial IDs;
- this repository must not invent new `CAP-*`/`SYS-*` semantics;
- one critical scenario may require multiple executable tests, and one well-structured system test may prove several explicitly mapped IDs.

## 5. Determinism and real boundaries

### Direct MCP v5 governance verification

The current implementation, Android handoff and executed candidate evidence are recorded in [Direct MCP tool governance §7](direct-mcp-tool-governance.md). Contract/hash-vector, Hub discovery/draft/workspace and HTTP boundary tests cover ALL/ALLOWLIST, binding removal, nonempty restrictions and disabled-source preservation. The editors verify discovery failure, explicit drift review, search clearing, default ALL on binding and no silent widening when an allowlist is emptied. The browser harness adds `mcp-tool-governance.spec.ts` after authoring and before runtime traffic, against the same production Admin, Hub/Relay and deterministic MCP.

The invocation-policy correction and current evidence are recorded in §8 of that document. Editor tests observe Red before changing new grants to AUTO, retaining explicit policies on contract review and removing the ALL confirmation claim. The real browser verifies mixed AUTO/REQUIRE_CONFIRMATION policies through save/reopen/preview/publish before checking unrestricted dynamic discovery. Native enforcement remains an Android consumer gate.

The current response-extension rules and Red/Green evidence are recorded in [protocol-compatibility.md](protocol-compatibility.md). Supported responses accept unknown fields recursively; commands and known execution semantics retain validation. Snapshot output non-disclosure is a producer gate, independent of consumer extension tolerance. Shared receiver cases cover both v4/v5 and nested stable-control extensions; Core/Portal evidence does not prove an installed Android decoder has adopted the rule.

The catalog UX follow-up covers read-only collapsed ALL catalogs, explicitly scoped bulk selection, preserving policies and choices outside search, and clearing changed tools by their visible current descriptions. Assistant search matches descriptions and retains choices outside the results. A 27-tool deterministic catalog includes long multi-line descriptions and an unusually long name; the production browser verifies bounded rows, full-definition details that do not select tools, compact multiple selections, and desktop/320px layouts. It supplements the 64-tool paging lane; screenshots show actual Admin rendering with synthetic catalog data.

The assistant picker now uses an explicit server-labelled dialog with checkboxes, selected/not-selected badges and a selected-only filter. Tests cover missing selections that can be removed but not newly granted, closing without silent mode changes, and preserving selections across pages. The browser pins a verified copy of the production SPA inside its isolated run directory; rebuilding the shared dist cannot delete the entry page or mix chunks during the run. `e2e-admin-build.json` binds the served snapshot to its build hash. Snapshot-copy tests require exact bytes, reject absent entry pages and prevent reuse of an existing run snapshot.

`node scripts/checks.mjs fmt` ignores checkout-only CRLF differences after comparing actual gofmt output; it still rejects formatting drift and never rewrites files. Portal independently runs generation, typecheck/unit/build and both STANDARD/CUSTOM real browser lanes. Android consumer/device evidence remains separate and pending for this increment.

`node scripts/e2e-harness.mjs --keep --manual` pauses after authoring and writes synthetic local test credentials in the isolated temp directory; use the real Admin UI for manual checks, then press Enter to continue the automated MCP/runtime/usage/topology phases. Failure injection lives only in the deterministic adapter. Manual screenshots and automatic 320px screenshots are local `.artifacts` evidence. This lane does not prove Android adoption or a new stage Freeze; its final report records dirty source/architecture state.

### Browser candidate execution

`make s01-browser-candidate` builds the production Admin SPA and runs `node scripts/e2e-harness.mjs` with isolated SQLite, ports, keys, Hub/Relay processes and a deterministic Adapter. `golden-path-authoring.spec.ts` configures and publishes through the browser; the Test Client generates runtime traffic in the same environment; `golden-path-usage.spec.ts` then checks Usage/System. `topology-security.spec.ts` covers the public/private boundary. Do not combine results from unrelated databases into one business-flow claim. Playwright JSON defaults to `.artifacts/e2e-playwright.json`; failures retain traces. For a failure, record the exact commit, command, browser version, failing request/response and process teardown result before changing a timeout or assertion. This lane is browser evidence, not Android device or real supplier qualification.

Every automated deterministic test must use isolated temp data/ports, avoid order dependency, default to no public-network access, use synthetic credentials, bound asynchronous waits and clean up processes/files.

Node HTTP harnesses allocate loopback ports through `scripts/lib/harness.mjs` and exclude Fetch-blocked ports. An OS-assigned free port can still be unusable by Node fetch or a browser, especially with a customized Windows dynamic port range. Allocation closes each probe and fails after 100 incompatible candidates; it does not change host networking or browser security settings. `scripts/harness-network.test.mjs` verifies blocked-port selection, bounded failure and an actual local HTTP fetch.

Do not mock away the behavior under test:

- Hub persistence tests use real SQLite;
- Relay spool tests use real SQLite;
- Relay streaming/cancellation tests use real HTTP/TCP boundaries;
- schema tests execute the ordered embedded migration set;
- Admin static-host tests use production build output;
- T3 Hub/Relay tests run real processes/binaries;
- S0.3 T3/T4.3 tests run real Hub/Gateway/Relay production binaries plus deterministic downstream MCP;
- S0.1 browser/system tests use real Admin→Hub→Relay paths;
- Test Client uses public Client Control + Runtime paths, not internal shortcuts.

Mocks/fakes are appropriate for uncontrollable third-party services, clocks/randomness and targeted failure injection. The deterministic Adapter is an intentional external-boundary test service, not a substitute for Hub/Relay.

## 6. Coverage, failures and flakiness

There is no global percentage that substitutes for architecture Testing Specs. Coverage reports are diagnostics only; missing required scenarios, failure/security cases or regression tests block completion regardless of line coverage.

A product/test assertion failure remains a failure. Do not retry tests until Green to mask defects. A whole CI job may be rerun once for a clearly identified runner/infrastructure failure, with the rerun remaining visible evidence.

Flaky critical tests are defects and must be fixed rather than permanently quarantined.

## 7. PR CI design

Default PR CI stops at T3 and does **not** execute Playwright/browser T4.1.

Current `.github/workflows/ci-gate.yml` jobs (PR to main / push to main):

```text
static-contract
backend-test
console-test
system-test       # bounded deterministic T3 only
ci-gate
```

`ci-gate` is the merge-facing aggregate check and must always be reported for pull requests. A Green `ci-gate` means the repository's normal T0–T3 baseline is Green; it does **not** mean C6, C7, S0.1 Freeze or final S0 Exit is complete.

The required gate evaluates the latest PR commit. Older Green checks are historical regression evidence only.

`make ci` runs real tests only: the static job runs `npm run test:tooling` and `make fmt-check contract`, and the aggregate adds backend, system and console tests. Regeneration and drift comparison are local commands (`make generate`, `make drift`, `make collect-static-contract`) and are deliberately **not** part of CI, so a Green `ci-gate` does **not** prove that generated output matches its source: a clean Git diff without regeneration proves nothing. Regenerate, inspect and commit derived output before freezing. The static job also runs Node evidence/tooling regression tests, including the command-line and freeze-producer contract checks; console typecheck uses vue-tsc for Vue templates. `make system-test` uses `-tags=smoke`; ordinary `go test ./...` excludes both smoke and candidate scenarios. Exact direct commands are in [development](development.md).

## 8. Explicit S0.1 candidate verification

Promotion to an S0.1 C6/C7 candidate is a separate explicit verification run on the exact candidate SHA. It must execute the architecture-defined T4.1 `CAP-*` gate with production Admin, real Hub/Relay, deterministic Adapter/Test Client and required recovery/security scenarios.

The runner may be developer-controlled/local or a dedicated controlled environment, but it must be reproducible and record exact source/build identity, timestamps, scenario results and safe diagnostics. Historical E2E evidence cannot be reused after the candidate SHA changes unless architecture explicitly allows it.

Real external Adapter qualification is a separate explicit lane and is not replaced by deterministic Adapter tests.

## 9. Freeze evidence

A **final accepted** S0.1 manifest requires all applicable candidate scenarios and real Adapter qualification, including replay. The writer first produces a draft with CAP-C7-002=NOT_EXECUTED. Independent clean-source replay must rebuild the pinned composition and execute the required path before separate finalization can set C7 to PASS. The CAP runner pins Snapshot v5 resource evidence and requires the CAP-C0-010 selectors in `scripts/scenario-definitions.json`: shared/default wire cases, current policy compilation, golden unset-default hash, six enabled-model references, attachment IMAGE admission and `TestStarterV5StrictWireAndV4Isolation`. This proves defaults and the v4/v5 wire boundary, including required opening; it does not replace Starter authoring/consumer checks or the S0.2 ERX gate. A draft is not a Freeze. Preserve historical evidence without labeling it current; see [release](release.md) for provenance and commands.

The architecture System Testing Spec is authoritative for the manifest fields. Current required identities include at least:

```text
architectureCommit
platformCoreCommit
adminBuildHash
clientControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
deterministicAdapterVersion
realAdapterQualificationRef
scenarioResults
startedAt
completedAt
```

`docs/release.md` defines how the implementation packages this evidence; the harness owns the exact serialized schema. Do not create competing hand-maintained manifest schemas in multiple Markdown files.

## 10. Historical evidence

Historical audit/test mapping from older architecture baselines remains useful as regression evidence, but it is not a living status document and must never be used to infer that a newer architecture checkpoint is Green.

The fixed S0.2 Preview composition and its evidence boundary are summarized in `docs/s0-execution-progress.md`. A future checkpoint needs its own fixed source, build and execution record.

The [S0.2 status](s0-execution-progress.md) does not substitute for per-scenario device records. A script exit status or stored PASS field alone remains insufficient for Freeze acceptance.

## 11. TDD cycle

This repository uses TDD for new behavior and regression fixes. Architecture defines the required behavior; executable tests in this repository prove it.

```text
Requirement
  ↓
Red      — write/enable the smallest executable test that fails for the intended reason
  ↓
Green    — implement the minimum behavior that satisfies the test
  ↓
Refactor — improve design while all relevant tests remain Green
  ↓
Gate     — run the complete affected CI/component/system layer
```

TDD is not "write tests eventually." The failure must be observed before the implementation is considered proven by that test.

### What counts as valid Red

A useful Red test:

- describes a real architecture requirement, regression or implementation invariant;
- fails before the production change;
- fails for the expected behavioral reason;
- is deterministic;
- remains in the final codebase.

Prefer an assertion failure that demonstrates missing/wrong behavior. A compile failure can be a transient Red signal when introducing a new interface, but it is weaker evidence if it does not demonstrate the behavior being built.

Do not create meaningless tests solely to manufacture a Red commit.

### Feature TDD

For a new capability already authorized by architecture:

1. identify the architecture requirement and applicable critical scenario ID;
2. choose the lowest test layer that can prove the next behavior slice;
3. add the failing test/fixture;
4. observe Red;
5. implement the smallest slice;
6. observe Green;
7. refactor;
8. add higher-layer integration/system proof only when the slice crosses real boundaries.

Do not start with a huge T4 test when a T1/T2 test can drive the internal behavior. Build outward.

### Regression TDD

For a bug:

```text
reproduce with a test
→ confirm Red on the buggy implementation
→ fix
→ confirm Green
→ run affected regression/component/system gate
```

If the bug exposes a missing architecture scenario, update the relevant Testing Spec and reference its stable ID. The local test does not become a new semantic authority by itself.

### Cross-component TDD

Cross-component work should use two loops:

**Inner loop** — Drive component behavior with T1/T2 tests using controlled peers/fakes.

**Contract/system loop** — Then prove the real boundary with T3/T4:

```text
contract/fixture Red
→ component implementation Green
→ real Hub/Relay/Admin integration
→ mapped SYS scenario Green
```

Do not require the full system harness for every tiny domain edit, and do not stop at mocks when architecture requires real-process integration.

## 12. Local TDD

With a local checkout:

1. add test;
2. run the narrow test and observe Red;
3. implement;
4. rerun and observe Green;
5. refactor;
6. run affected component suite;
7. push and let GitHub CI independently reproduce the result.

Local execution optimizes feedback time; GitHub CI provides merge evidence. Neither replaces the other when both are available.

## 13. GitHub-only TDD

A fully GitHub-based Red/Green loop is valid when GitHub Actions is the execution environment.

### Required sequence

1. Create a feature/fix branch.
2. Open a **Draft PR before the Red commit is evaluated** so `pull_request` workflows run and evidence is attached to the PR.
3. Add the failing test in a Red commit.
4. Let GitHub Actions execute the relevant job.
5. Inspect the failed check/job log and verify that the intended test failed for the expected reason.
6. Record the Red commit SHA/check in the PR description when the change is non-trivial.
7. Add the minimum production implementation in a later commit.
8. Let Actions run on the new head SHA.
9. Verify the affected checks are Green on the latest commit.
10. Refactor in additional commits as needed; Actions must remain Green.
11. Merge only after the complete required aggregate gate passes.

### Why this is verifiable

GitHub Actions creates check runs for workflow jobs. Protected branches can require those checks to pass before merge, and required checks apply to the current PR head rather than an older successful commit. Test reports/logs can be retained as workflow artifacts.

Therefore a developer/agent without a local runtime can still produce auditable evidence:

```text
PR
├── Red commit SHA → failing Actions check/log
├── Green commit SHA → successful Actions check
└── final SHA → required ci-gate success + artifacts
```

### Remote-only agent rule

An agent that can edit GitHub but cannot execute locally must never say "tests pass" based only on reading code. It must inspect the GitHub Actions run/check for the relevant commit. If the repository lacks CI for the required test, the result is **not verified** and the missing CI capability should be implemented or reported.

A T4.1/browser requirement cannot be declared Green from default GitHub Actions; it requires the explicit candidate run described in §8.

## 14. PR evidence format

For non-trivial behavior changes, include:

```text
Architecture: <document/section/scenario ID>
Red: <commit SHA> / <test name> / expected failure
Green: <commit SHA or latest> / <checks passed>
Additional gates: <T0/T1/T2/T3/T4 lanes>
```

A screenshot is optional and weaker than a check/run link + commit SHA.

## 15. Refactoring

Refactoring begins from Green. It should not require changing expected behavior. If a refactor forces behavior expectations to change, it is no longer a pure refactor and must be reclassified.

Use characterization tests first when refactoring poorly understood existing behavior.

## 16. TDD exceptions

Do not force artificial Red/Green commits for:

- documentation-only edits;
- formatting-only changes;
- deterministic regeneration where source semantics did not change;
- dependency lockfile refresh with no intended behavior change.

These changes still run applicable static/build/contract gates.

Schema changes are not exempt: use repository schema tests that fail before the current initialization behavior exists.

## 17. TDD anti-patterns

Forbidden TDD shortcuts:

- writing implementation and test together, then claiming an unobserved theoretical Red;
- disabling the test during implementation;
- adding retries to hide product failures;
- asserting internal implementation details when the requirement is observable behavior;
- using only mocks for a required real-boundary scenario;
- deleting a regression test once the fix is Green;
- changing architecture semantics inside the test to make implementation convenient.

## 18. Merge policy target

After I0 CI is live, configure `main` so changes merge through PRs and the stable aggregate CI check is required. The required check should run for every PR rather than being suppressed by workflow-level path filters.

This turns TDD from a convention into an enforceable development loop: Red may exist on the branch, but Green is required before merge.

## 19. Secrets and artifacts

No production token, Secret, enrollment code, refresh credential, real conversation or sensitive prompt may appear in fixtures, logs or artifacts. Security scenarios additionally assert that protected material does not appear in responses, DOM/persistent browser state, managed Snapshot or usage events.

## 20. Starter v5 cross-consumer verification

Direct MCP 的定向与浏览器验证入口见 [工具治理实施](direct-mcp-tool-governance.md#验证入口与边界)。`node scripts/e2e-harness.mjs` 顺序运行 authoring、MCP、五类 Runtime、usage、topology 和 Admin 全路由 review，使用生产 SPA 与真实 API，并固定本次服务的构建副本和摘要。Android lane 仅在显式指定设备时执行。

`MEASIX_E2E_ANDROID_SERIAL=emulator-5562 node scripts/e2e-harness.mjs` adds an opt-in native lane after the real Admin authoring/publish flow. Install the matching debug and androidTest APKs on a dedicated, fresh unbound emulator first; the harness never installs or clears app data and rejects the retained production-demo emulator. `ADB` may select the executable. The public Admin API issues temporary enrollment, adb reverse preserves canonical origin, and Android performs the actual HTTP sync, UI prefill/send, Room readback and context-detail reopen. The deterministic adapter verifies the exact published opening and absence of an additional Assistant System. Only synthetic request bodies are captured; transient enrollment files and reverse mapping are removed in finally. This is local Core/Android evidence, not a production-model or physical-device gate.

Use `npm run test:tooling` for instrumentation-result and wire-verifier negative cases, and the normal Go/Console gates for old-release preservation and authoring conflicts. Evidence and supported data boundaries are recorded in [Starter opening snapshots](starter-opening-snapshots.md).

`device:real` adds a separate actual-supplier lane. Its preset must contain complete v5 openings, fail closed on validation/migration errors, and preserve existing database and Secret identities. Tooling regressions cover these behaviors. Readiness/ACTIVE and connectivity tests do not prove successful model invocation. Record credential/entitlement failures separately from deterministic end-to-end results. Correlate the browser-authored opening, release/hash, device Applied report and response/error; credentials never enter evidence. The Starter verification document records the current run boundary.

`scripts/real-device-preset.test.mjs` also covers a restart after Admin discovery: full-draft replacement preserves the matching server-owned catalog, including raw Tool extension fields, and keeps the preset's explicit ALL scope without a discovery prerequisite. Failure diagnostics retain endpoint/status/code, exclude response bodies, and never reuse a previous attempt's result. Successful output reports the completed activation's generation; an unchanged preset does not publish another release. Build-identity cases refuse mutations against an older Hub or Relay and wait for a starting Relay to report its identity.



## 远程工作区专项验证

`node scripts/workspace-integration.mjs --config <local-config.json>` 使用独立固定 Agent Space 服务和全新 Core 数据库执行真实双用户 MCP/DAV、预算、64 MiB 传输、取消、生命周期、重启和删除。浏览器审查另行操作生产构建页面，脚本通过不代替 UI 验收。

详见 [远程工作区实现参考](remote-workspace-implementation.md) 与 [当前联调记录](remote-workspace-verification.md)。
