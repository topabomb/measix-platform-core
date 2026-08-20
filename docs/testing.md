# Testing and CI

This document defines how `measix-platform-core` executes and records tests. Required behavior and critical scenarios remain authoritative in the S0.1/S0.2/Component/System Testing Specs in `topabomb/measix-architecture`.

## 1. Test layers

The repository implements the S0 five-layer model:

| Layer | Purpose | Repository implementation |
|---|---|---|
| T0 Static / Contract | schema, codegen, migration, fixture, build consistency | API validation, generated drift, migration replay, production build |
| T1 Unit / Domain | pure validation/state/mapping | Go unit tests, Vitest unit/store helpers |
| T2 Component Integration | one real component + local real boundaries | real SQLite, real HTTP server, component/static-host tests |
| T3 Cross-component Integration | multiple real MEASIX components | real Hub↔Relay, Admin↔Hub, deterministic Adapter |
| T4 System / E2E | client-facing topology and full product flows | S0.1 pre-Android explicit candidate gate, then final S0.2 Android/system RC |

Normal GitHub Actions CI is deliberately limited to deterministic T0–T3 plus required builds. Browser/System E2E is not executed by GitHub Actions CI/CD because it is materially slower and is a promotion/freeze proof rather than a per-commit feedback loop. S0.1 still requires the complete deterministic pre-Android T4.1 gate before C6/C7 completion. Final S0 RC still requires Android emulator/device and the remaining cross-repository requirements from the architecture System Testing Spec.

## 2. Delivery-gate mapping

### S0 Core evidence

Existing I0–I5 tests remain valuable regression evidence for identity, Draft/Snapshot foundations, Relay admission/transport, metering, persistence and Admin infrastructure. They do **not** by themselves prove S0.1 or final S0 Exit.

### S0.1 Managed Capability Delivery

S0.1 is a pre-Android product/system gate owned semantically by `measix-s0-capability-delivery-system-testing-spec.md`.

Its executable topology is:

```text
real Admin browser
  → real Control Hub
  → real Runtime Relay
  → deterministic Test Adapter / qualified real Adapter

Snapshot/Runtime Test Client
  → real Client Control API + Runtime API
```

S0.1 must prove the architecture `CAP-*` scenarios, including the required Managed Capability profiles, Snapshot preview/release equivalence, publish/runtime enforcement, usage/pricing/diagnostics, no-forward security behavior and the Client Contract Freeze evidence.

No Android commit or emulator is required for the S0.1 gate.

### S0.2 / final S0 Exit

S0.2 consumes the pinned S0.1 freeze manifest. Final S0 RC adds the real `rikkahub_mcp` Android implementation and proves the architecture `AND-*`/`SYS-*` scenarios with fixed cross-repository source commits.

A passing S0.1 gate must never be reported as final S0 Exit.

## 3. Test locations

Target conventions:

```text
backend/**/**/*_test.go          Go unit/component tests
console/**/*.spec.ts            frontend unit/component tests
api/fixtures/                   canonical contract fixtures
test/qualification/             Adapter qualification harness
test/system/harness/            real-process system harness
test/system/scenarios/          CAP/SYS scenario orchestration
test/system/reports/            generated candidate output/evidence; not a normal CI artifact lane
```

Tests should live near the code they prove unless they are explicitly cross-component/system infrastructure.

## 4. Mapping architecture requirements

Critical architecture scenarios use stable IDs such as `HUB-*`, `RLY-*`, `ADM-*`, `CAP-*`, `AND-*` and `SYS-*`.

Rules:

- a test proving a critical scenario must expose the ID in its test name, metadata or nearby comment so reports can map it;
- ordinary unit tests do not need artificial IDs;
- this repository must not invent a new `CAP-*`/`SYS-*` semantic; add it to the architecture Testing Spec first;
- one critical scenario may require several executable tests, and one well-structured system test may prove several explicitly listed IDs.

## 5. Determinism and isolation

Every automated test must:

- use its own temp directory/database/ports/identity;
- avoid dependency on test order;
- default to no public-network access;
- use synthetic credentials and content;
- control clock/randomness when behavior depends on them;
- use bounded eventually assertions rather than fixed long sleeps;
- clean up processes/files or preserve them only as explicit failure artifacts.

Critical deterministic tests cannot depend on a flaky external Provider. Real Adapter qualification is a separate S0.1/RC lane.

## 6. Real-boundary rules

Do not mock away the behavior under test:

- Hub persistence tests use real SQLite files;
- Relay spool tests use real SQLite files;
- Relay streaming/cancellation tests use real HTTP/TCP boundaries;
- migration tests execute versioned SQL;
- Admin static-host tests use real production build output;
- T3 Hub/Relay tests run real processes/binaries;
- S0.1 browser/system tests use real Admin→Hub→Relay paths;
- S0.1 Test Client uses the public Client Control and Runtime paths, not internal shortcuts;
- final S0 T4 uses the real Android client-facing URL topology.

Mocks/fakes are appropriate for uncontrollable third-party services, clocks/randomness, and targeted failure injection.

## 7. Coverage policy

There is no global percentage that substitutes for the architecture Testing Specs.

Coverage reports may be generated as diagnostics and trend signals. A PR fails for missing required scenarios, contract/failure/security coverage or regression tests even when line coverage is high.

Do not add a repository-wide percentage gate without a specific engineering reason and review; if introduced, it remains secondary to required scenarios.

## 8. Failure, retry and flakiness

A product/test assertion failure must remain a failure. Tests do not retry until green.

A whole CI job may be rerun once for a clearly identified runner/infrastructure failure, but the rerun is visible evidence and must not mask reproducible product failures.

Flaky critical tests are defects. Fix them; do not permanently quarantine/skip them.

## 9. PR CI design

GitHub Actions should expose stable, understandable checks and stay fast enough to remain useful on every push. The default PR gate therefore stops at T3 and does **not** execute Playwright/browser T4.1 E2E.

Recommended logical jobs are:

```text
static-contract
unit
component-hub
component-relay
component-admin
system-test       # bounded deterministic T3 only
ci-gate
```

`ci-gate` is the merge-facing aggregate check and **must always be reported for pull requests**. Do not put workflow-level path filters on a required check: a required workflow that never runs can leave a PR waiting for a status indefinitely. Component jobs may skip work internally when unaffected, but the aggregate result remains explicit.

The required gate evaluates the latest PR commit. Older Green checks do not satisfy a new head commit. A Green `ci-gate` means the repository's normal T0–T3 CI baseline is Green; it does **not** mean C6, C7, S0.1 Freeze or final S0 Exit is complete.

## 10. Main/integration CI and explicit E2E promotion

After merge or on an integration candidate, GitHub Actions runs at least:

- all PR gates;
- Hub↔Relay T3;
- Admin↔Hub T3 where implemented as bounded integration tests;
- migration replay/upgrade;
- Admin production static host/build;
- deterministic Adapter transport suite;
- core backend/system-harness T3 scenarios.

GitHub Actions CI/CD intentionally does **not** run T4.1 browser/system E2E, even on `main`. This keeps continuous feedback deterministic and bounded.

Promotion to an S0.1 C6/C7 freeze candidate adds a separate explicit candidate verification outside GitHub Actions CI/CD. It must run on the exact candidate commit and execute the complete architecture-defined `CAP-*` T4.1 gate with production Admin, real Hub/Relay, deterministic Adapter/Test Client, and the required recovery/security scenarios. The candidate is blocked if this run is missing or not Green, even when GitHub Actions `ci-gate` is Green.

The candidate E2E runner may be a developer-controlled/local or dedicated controlled environment, but it must be reproducible and record at least the exact source SHA, command/config identity, started/completed timestamps, scenario results and safe diagnostics. Historical E2E evidence cannot be reused after the candidate SHA changes unless the architecture gate explicitly allows an unaffected artifact, which S0.1 Freeze currently does not.

Real external Adapter qualification remains a separate explicit qualification lane and is also not part of normal GitHub Actions CI/CD.

## 11. S0.1 freeze evidence

The explicit S0.1 candidate system run generates machine-readable evidence and diagnostics containing no secrets.

The freeze manifest pins at least:

```text
architectureCommit
platformCoreCommit
clientControlOpenApiHash
canonicalFixtureHash
snapshotSchemaVersion
adminBuildHash
adapterQualificationRef
scenarioResults
startedAt/completedAt
```

The exact serialized schema is implemented by the harness and follows the architecture contract; this document does not create a second wire schema.

The manifest exists only after all required S0.1 scenarios are Green. A document or commit message saying “frozen” is not a substitute.

## 12. Final S0 RC evidence

Final T4/RC extends the frozen S0.1 composition with fixed Android identity and the additional architecture-required evidence, for example:

```text
platformCoreCommit
androidCommit
architectureCommit
adminBuildHash
openApiFixtureHash
hubBuild
relayBuild
adapter qualification identity
androidAppVersion
scenarioResults
startedAt/completedAt
```

The final architecture System Testing Spec is authoritative for the exact required fields/scenarios.

## 13. Local test loop

Local development uses the narrowest test that demonstrates Red/Green, then expands to the affected component suite before push. CI repeats the required T0–T3 gate independently.

When a change affects an existing browser/system workflow, run the smallest affected E2E slice explicitly during development where practical; this is development evidence, not a reason to move the full T4.1 suite into Actions. Before C6/C7 promotion, always rerun the complete candidate gate on the exact candidate SHA.

Exact commands are documented here only when their actual scripts/package targets exist. Do not invent documentation-only commands that CI cannot execute.

## 14. GitHub-only test loop

When no local runtime is available:

1. open/use a Draft PR;
2. push the Red test commit;
3. wait for/inspect the Actions check;
4. confirm the intended T0–T3 test failed for the expected reason;
5. push the Green implementation commit;
6. inspect checks on the **latest commit SHA**;
7. inspect job logs/artifacts for the affected CI lane;
8. refactor and repeat if needed.

This is a valid TDD loop for T0–T3 because execution is delegated to CI rather than skipped. A T4.1/browser requirement cannot be declared Green from GitHub-only CI; it requires the separate explicit candidate E2E run. See `docs/tdd.md`.

## 15. Secrets and test data

No production token, secret, enrollment code, refresh credential, real conversation or sensitive prompt is allowed in fixtures, logs or artifacts.

Security scenarios additionally assert that protected material does not appear in responses, logs, reports, Admin storage/DOM, managed Snapshot or usage events.

## 16. Test-change review

Deleting or weakening a test requires a reason. A critical test may be removed only when the architecture capability/scenario is removed or another test demonstrably covers the same requirement. “Hard to test” is not sufficient.
