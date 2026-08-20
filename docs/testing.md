# Testing and CI

This document defines how `measix-platform-core` executes and records tests. Required behavior and critical scenarios remain authoritative in the S0.1/S0.2/Component/System Testing Specs in `topabomb/measix-architecture`.

## 1. Test layers

| Layer | Purpose | Repository implementation |
|---|---|---|
| T0 Static / Contract | schema, codegen, migration, fixture, build consistency | API validation, generated drift, migration replay, production build |
| T1 Unit / Domain | pure validation/state/mapping | Go unit tests, Vitest unit/store helpers |
| T2 Component Integration | one real component + local real boundaries | real SQLite, real HTTP server, component/static-host tests |
| T3 Cross-component Integration | multiple real MEASIX components | real Hub↔Relay, Admin↔Hub where implemented, deterministic Adapter |
| T4.1 S0.1 Product/System E2E | pre-Android product topology | production browser + real Hub/Relay + Adapter + Test Client |
| T4 Final S0 System / RC | frozen S0.1 + Android | cross-repository Android/system RC |

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

### S0.2 / final S0 Exit

S0.2 consumes the pinned S0.1 freeze. Final S0 RC adds real `rikkahub_mcp` Android and proves applicable `AND-*`/`SYS-*` scenarios with fixed cross-repository commits. A passing S0.1 gate must never be reported as final S0 Exit.

## 3. Current test locations

Concrete current paths are:

```text
backend/**/**/*_test.go          Go unit/component tests
console/**/*.spec.ts            frontend unit/component tests
api/fixtures/                   canonical contract fixtures
backend/test/system/            current Go-module deterministic/system harness
console/e2e/                    browser E2E ownership when executable Playwright tests land
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

Critical architecture scenarios use stable IDs such as `HUB-*`, `RLY-*`, `ADM-*`, `CAP-*`, `AND-*` and `SYS-*`.

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
- S0.1 browser/system tests use real Admin→Hub→Relay paths;
- Test Client uses public Client Control + Runtime paths, not internal shortcuts.

Mocks/fakes are appropriate for uncontrollable third-party services, clocks/randomness and targeted failure injection. The deterministic Adapter is an intentional external-boundary test service, not a substitute for Hub/Relay.

## 6. Coverage, failures and flakiness

There is no global percentage that substitutes for architecture Testing Specs. Coverage reports are diagnostics only; missing required scenarios, failure/security cases or regression tests block completion regardless of line coverage.

A product/test assertion failure remains a failure. Do not retry tests until Green to mask defects. A whole CI job may be rerun once for a clearly identified runner/infrastructure failure, with the rerun remaining visible evidence.

Flaky critical tests are defects and must be fixed rather than permanently quarantined.

## 7. PR CI design

Default PR CI stops at T3 and does **not** execute Playwright/browser T4.1.

Recommended logical jobs:

```text
static-contract
unit
component-hub
component-relay
component-admin
system-test       # bounded deterministic T3 only
ci-gate
```

`ci-gate` is the merge-facing aggregate check and must always be reported for pull requests. A Green `ci-gate` means the repository's normal T0–T3 baseline is Green; it does **not** mean C6, C7, S0.1 Freeze or final S0 Exit is complete.

The required gate evaluates the latest PR commit. Older Green checks are historical regression evidence only.

## 8. Explicit S0.1 candidate verification

Promotion to an S0.1 C6/C7 candidate is a separate explicit verification run on the exact candidate SHA. It must execute the architecture-defined T4.1 `CAP-*` gate with production Admin, real Hub/Relay, deterministic Adapter/Test Client and required recovery/security scenarios.

The runner may be developer-controlled/local or a dedicated controlled environment, but it must be reproducible and record exact source/build identity, timestamps, scenario results and safe diagnostics. Historical E2E evidence cannot be reused after the candidate SHA changes unless architecture explicitly allows it.

Real external Adapter qualification is a separate explicit lane and is not replaced by deterministic Adapter tests.

## 9. Freeze evidence

A valid S0.1 freeze manifest is generated **only after** all required candidate scenarios and real Adapter qualification are Green. The repository must not retain a placeholder/stale manifest as current evidence.

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

`docs/s0-review-report.md` records historical audit/test mapping tied to the architecture/core baseline stated inside that report. It remains useful regression evidence, but it is not a living status document and must never be used to infer that a newer architecture checkpoint is Green.

Current checkpoint status lives only in `docs/s0-execution-progress.md`, backed by executable results for the current architecture baseline and current implementation SHA.

## 11. TDD and local/GitHub-only loops

Local development uses the narrowest test that demonstrates Red/Green, then expands to the affected component suite before push. GitHub CI independently repeats required T0–T3 gates.

GitHub-only development is valid for T0–T3 when Actions executes the Red and Green commits and the relevant logs/checks are inspected. Static code review alone is not execution.

A T4.1/browser requirement cannot be declared Green from default GitHub Actions; it requires the explicit candidate run described above. See `docs/tdd.md`.

## 12. Secrets and artifacts

No production token, Secret, enrollment code, refresh credential, real conversation or sensitive prompt may appear in fixtures, logs or artifacts. Security scenarios additionally assert that protected material does not appear in responses, DOM/persistent browser state, managed Snapshot or usage events.
