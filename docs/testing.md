# Testing and CI

This document defines how `measix-platform-core` executes and records tests. Required behavior and critical scenarios remain authoritative in the S0 Component/System Testing Specs in `topabomb/measix-architecture`.

## 1. Test layers

The repository implements the S0 five-layer model:

| Layer | Purpose | Repository implementation |
|---|---|---|
| T0 Static / Contract | schema, codegen, migration, fixture, build consistency | API validation, generated drift, migration replay, production build |
| T1 Unit / Domain | pure validation/state/mapping | Go unit tests, Vitest unit/store helpers |
| T2 Component Integration | one real component + local real boundaries | real SQLite, real HTTP server, component/static-host tests |
| T3 Cross-component Integration | multiple real MEASIX components | real Hub↔Relay, Admin↔Hub, deterministic Adapter |
| T4 S0 System / E2E | client-facing topology and full S0 flows | system harness + fixed Android repository commit |

Normal PR CI is dominated by T0/T1/T2. Main/integration adds required T3. RC adds complete T4 and real Adapter/Android/browser requirements from the architecture System Testing Spec.

## 2. Test locations

Target conventions:

```text
backend/**/**/*_test.go          Go unit/component tests
console/**/*.spec.ts            frontend unit/component tests
api/fixtures/                   canonical contract fixtures
test/qualification/             Adapter qualification harness
test/system/harness/            real-process system harness
test/system/scenarios/          SYS scenario orchestration
test/system/reports/            generated local output; CI publishes artifacts
```

Tests should live near the code they prove unless they are explicitly cross-component/system infrastructure.

## 3. Mapping architecture requirements

Critical architecture scenarios use stable IDs such as `HUB-*`, `RLY-*`, `ADM-*`, `SYS-*`.

Rules:

- a test proving a critical scenario must expose the ID in its test name, metadata or nearby comment so reports can map it;
- ordinary unit tests do not need artificial IDs;
- this repository must not invent a new `SYS-*` semantic; add it to the architecture Testing Spec first;
- one critical scenario may require several executable tests, and one well-structured system test may prove several explicitly listed IDs.

## 4. Determinism and isolation

Every automated test must:

- use its own temp directory/database/ports/identity;
- avoid dependency on test order;
- default to no public-network access;
- use synthetic credentials and content;
- control clock/randomness when behavior depends on them;
- use bounded eventually assertions rather than fixed long sleeps;
- clean up processes/files or preserve them only as explicit failure artifacts.

Critical tests cannot depend on a flaky external Provider. Real Adapter qualification is a separate RC lane.

## 5. Real-boundary rules

Do not mock away the behavior under test:

- Hub persistence tests use real SQLite files;
- Relay spool tests use real SQLite files;
- Relay streaming/cancellation tests use real HTTP/TCP boundaries;
- migration tests execute versioned SQL;
- Admin static-host tests use real production build output;
- T3 Hub/Relay tests run real processes/binaries;
- T4 uses the client-facing URL topology, not internal shortcuts.

Mocks/fakes are appropriate for uncontrollable third-party services, clocks/randomness, and targeted failure injection.

## 6. Coverage policy

There is no global percentage that substitutes for the architecture Testing Specs.

Coverage reports may be generated as diagnostics and trend signals. A PR fails for missing required scenarios, contract/failure/security coverage or regression tests even when line coverage is high.

Do not add a repository-wide percentage gate without a specific engineering reason and review; if introduced, it remains secondary to required scenarios.

## 7. Failure, retry and flakiness

A product/test assertion failure must remain a failure. Tests do not retry until green.

A whole CI job may be rerun once for a clearly identified runner/infrastructure failure, but the rerun is visible evidence and must not mask reproducible product failures.

Flaky critical tests are defects. Fix them; do not permanently quarantine/skip them.

## 8. PR CI design

Once I0 CI is implemented, GitHub Actions should expose stable, understandable checks. Recommended logical jobs are:

```text
static-contract
unit
component-hub
component-relay
component-admin
ci-gate
```

`ci-gate` is the merge-facing aggregate check and **must always be reported for pull requests**. Do not put workflow-level path filters on a required check: a required workflow that never runs can leave a PR waiting for a status indefinitely. Component jobs may skip work internally when unaffected, but the aggregate result remains explicit.

The required gate evaluates the latest PR commit. Older Green checks do not satisfy a new head commit.

## 9. Main/integration CI

After merge or on an integration candidate, run at least:

- all PR gates;
- Hub↔Relay T3;
- Admin↔Hub T3;
- migration replay/upgrade;
- Admin production static host;
- deterministic Adapter transport suite;
- core backend/system-harness scenarios.

A failure blocks promotion to RC even if `main` itself is not technically reverted automatically.

## 10. System/RC evidence

T3/T4 and RC runs publish machine-readable evidence and useful diagnostics as GitHub Actions artifacts. At minimum, retain the manifest required by architecture plus test reports/log summaries that contain no secrets.

The manifest includes fixed source identities such as:

```text
platformCoreCommit
androidCommit
architectureCommit
adminBuildHash
openApiFixtureHash
hubBuild
relayBuild
adapterName/version/configRevision
androidAppVersion
scenarioResults
startedAt/completedAt
```

The `architectureCommit` is implementation-governance metadata; it complements, not replaces, the System Testing Spec's required fixed source commits.

## 11. Local test loop

Local development uses the narrowest test that demonstrates Red/Green, then expands to the affected component suite before push. CI repeats the required gate independently.

Exact commands are documented here only when their actual scripts/package targets exist. Do not invent documentation-only commands that CI cannot execute.

## 12. GitHub-only test loop

When no local runtime is available:

1. open a Draft PR;
2. push the Red test commit;
3. wait for/inspect the Actions check;
4. confirm the intended test failed for the expected reason;
5. push the Green implementation commit;
6. inspect checks on the **latest commit SHA**;
7. inspect job logs/artifacts for the affected lane;
8. refactor and repeat if needed.

This is a valid TDD loop because execution is delegated to CI rather than skipped. See `docs/tdd.md`.

## 13. Secrets and test data

No production token, secret, enrollment code, refresh credential, real conversation or sensitive prompt is allowed in fixtures, logs or artifacts.

Security scenarios additionally assert that protected material does not appear in responses, logs, reports, Admin storage/DOM, managed Snapshot or usage events.

## 14. Test-change review

Deleting or weakening a test requires a reason. A critical test may be removed only when the architecture capability/scenario is removed or another test demonstrably covers the same requirement. “Hard to test” is not sufficient.
