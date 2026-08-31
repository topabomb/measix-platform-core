# Testing, CI and TDD

This document defines how `measix-platform-core` executes and records tests, and the TDD discipline that governs them. Required behavior and critical scenarios remain authoritative in the S0.1/S0.2/S0.3/S0.4/Component/System Testing Specs in `topabomb/measix-architecture`.

## 1. Test layers

| Layer | Purpose | Repository implementation |
|---|---|---|
| T0 Static / Contract | schema, codegen, migration, fixture, build consistency | API validation, generated drift, migration replay, production build |
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

Every automated deterministic test must use isolated temp data/ports, avoid order dependency, default to no public-network access, use synthetic credentials, bound asynchronous waits and clean up processes/files.

Do not mock away the behavior under test:

- Hub persistence tests use real SQLite;
- Relay spool tests use real SQLite;
- Relay streaming/cancellation tests use real HTTP/TCP boundaries;
- migration tests execute versioned SQL;
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

The static job explicitly runs `make generate` before drift checks; `make ci` alone does not. A clean Git diff without regeneration does not prove generated output matches source. `make system-test` uses `-tags=smoke`; ordinary `go test ./...` excludes both smoke and candidate scenarios. Exact direct commands are in [development](development.md).

## 8. Explicit S0.1 candidate verification

Promotion to an S0.1 C6/C7 candidate is a separate explicit verification run on the exact candidate SHA. It must execute the architecture-defined T4.1 `CAP-*` gate with production Admin, real Hub/Relay, deterministic Adapter/Test Client and required recovery/security scenarios.

The runner may be developer-controlled/local or a dedicated controlled environment, but it must be reproducible and record exact source/build identity, timestamps, scenario results and safe diagnostics. Historical E2E evidence cannot be reused after the candidate SHA changes unless architecture explicitly allows it.

Real external Adapter qualification is a separate explicit lane and is not replaced by deterministic Adapter tests.

## 9. Freeze evidence

A **final accepted** S0.1 manifest requires all applicable candidate scenarios and real Adapter qualification, including replay. The current writer/replayer is two-phase: a candidate draft can contain CAP-C7-002=NOT_EXECUTED, then replay may finalize it. That draft is not a Freeze. Preserve historical evidence without labeling it current; see [release](release.md) for provenance and known script limitations.

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

Current checkpoint status lives only in `docs/s0-execution-progress.md`, backed by executable results for the current architecture baseline and current implementation SHA.

The [2026-08-31 alignment audit](architecture-alignment-audit.md) records current source findings and bounded tests, not stage acceptance. Known gaps include collection working directories/error propagation, hardcoded manifest schemaVersion, incomplete provenance/profile validation, and Playwright trace/security flags. Until fixed and verified, script exit status or an existing PASS field alone is insufficient for Freeze acceptance.

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

Use characterization tests first when refactoring poorly understood legacy behavior.

## 16. TDD exceptions

Do not force artificial Red/Green commits for:

- documentation-only edits;
- formatting-only changes;
- deterministic regeneration where source semantics did not change;
- dependency lockfile refresh with no intended behavior change.

These changes still run applicable static/build/contract gates.

Migration/schema changes are not exempt: use repository/migration tests that fail before the schema/migration behavior exists.

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
