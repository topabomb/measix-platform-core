# Release and S0 Release-Candidate Verification

This document defines how an implementation candidate is composed and proven reproducibly. The architecture S0 System Testing Spec remains authoritative for what must pass before S0 can be declared complete.

## 1. Release principle

A release candidate is not “whatever is currently on main.” It is a fixed, reproducible composition of source commits, generated contracts, binaries/builds and test evidence.

For S0, the cross-repository composition at minimum fixes:

```text
measix-platform-core commit SHA
rikkahub_mcp commit SHA
```

The release manifest should also record the `measix-architecture` commit used as the governing architecture baseline.

## 2. Candidate manifest

The system/RC harness produces a machine-readable manifest containing at least the fields required by the architecture System Testing Spec. Implementation governance adds `architectureCommit` for traceability.

Expected shape conceptually:

```text
architectureCommit
platformCoreCommit
androidCommit
adminBuildHash
openApiFixtureHash
hubBuild
relayBuild
adapterName/version/configRevision
androidAppVersion
scenarioResults
startedAt/completedAt
```

The exact serialized schema belongs to the test harness once implemented; do not copy a second manually maintained schema into this document.

## 3. Promotion stages

### Pull request

T0/T1/T2 for the affected repository surface, with Red/Green evidence for behavior changes.

### Main / integration candidate

Adds required T3 lanes, migration/static-host checks and deterministic system-harness backend scenarios.

### S0 Release Candidate

Runs all required current-release S0 gates, including:

- applicable Component Testing Spec MUST scenarios;
- all applicable `SYS-*` critical scenarios;
- Android emulator E2E;
- Admin real-browser E2E;
- required real Adapter qualification;
- Hub/Relay restart/reconcile;
- backup/restore;
- usage spool/replay;
- target-resource/load validation;
- no unexplained critical test skip.

The architecture document is the authority for the exact S0 Exit list. This document only defines how the repository packages and preserves evidence.

## 4. Deterministic vs real-external lanes

Two distinct system lanes are maintained:

```text
Deterministic lane
  real MEASIX components
  deterministic Test Adapter
  no public Provider dependency

RC external qualification lane
  fixed Adapter version/config revision
  real required upstream capability
  qualification evidence
```

A flaky public Provider must not make normal PR CI nondeterministic. Conversely, a deterministic fake cannot be used to claim real Adapter qualification.

## 5. Build identity

Every RC binary/static build must be traceable to its source commit and build configuration. The exact build metadata mechanism is implemented in I0/I5 tooling, but release evidence must make it possible to determine which Hub, Relay and Admin build was tested.

## 6. GitHub Actions evidence

RC workflows publish:

- machine-readable manifest;
- test result files;
- bounded diagnostic logs;
- browser/emulator failure artifacts when useful;
- qualification reports;
- resource/load summaries.

Artifacts must not contain production credentials, real user conversations or secret plaintext.

A successful workflow status without the required scenario/report evidence is not sufficient for S0 RC.

## 7. Reproduction

An RC should be reproducible by checking out the pinned source commits, restoring the repository-controlled toolchain/lockfiles, rebuilding the artifacts, and rerunning the deterministic T4 harness with the same declared inputs.

Moving `main` branches are never used as implicit test inputs after the candidate manifest is created.

## 8. Failed candidate

When an RC scenario fails:

1. keep the failing run/artifact for diagnosis;
2. reproduce with the smallest relevant test where possible;
3. apply TDD regression workflow (Red already demonstrated by the failing scenario; add a narrower durable regression test when useful);
4. create a new candidate composition after fixes;
5. rerun affected gates and the full required RC gate before promotion.

Do not mutate the evidence of a failed candidate to make it look successful.

## 9. Tag/version policy

Concrete repository tag/release naming is intentionally not invented by this documentation baseline. Define and document it before the first real RC/release, then keep the naming rule here. Whatever naming is chosen, commit SHAs remain the reproducibility identity used by system-test manifests.
