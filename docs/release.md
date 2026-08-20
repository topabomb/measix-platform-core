# Release, S0.1 Freeze and Final S0 Release-Candidate Verification

This document defines how implementation candidates are composed and proven reproducibly. The architecture S0.1 Capability Delivery Testing Spec and final S0 System Testing Spec remain authoritative for what must pass.

## 1. Release principle

A candidate is not “whatever is currently on main.” It is a fixed, reproducible composition of source commits, generated contracts, builds and test evidence.

S0 now has two distinct evidence milestones:

```text
S0.1 Client Contract Freeze Candidate
  → pre-Android server-side product closure

Final S0 Release Candidate
  → pinned S0.1 contract + real Android S0.2 integration + final S0 Exit gate
```

Do not collapse these into one status.

GitHub Actions CI/CD is intentionally not the full release verifier. It provides the fast deterministic T0–T3 baseline. Browser/system E2E and real external qualification are explicit promotion gates executed separately on a pinned candidate SHA.

## 2. S0.1 freeze candidate

S0.1 is intentionally pre-Android. A freeze candidate fixes the server-side implementation and Android-facing executable contract after the architecture-defined S0.1 gate is Green.

The manifest must pin at least the identities required by the S0.1 architecture contract, including conceptually:

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

It does **not** require `androidCommit`, because Android S0.2 implementation starts only after this freeze.

The exact serialized manifest schema belongs to the system harness once implemented; do not copy a second manually maintained schema into this document.

## 3. Final S0 release candidate

Final S0 RC consumes a specific valid S0.1 freeze and adds the real Android implementation.

The cross-repository composition at minimum fixes:

```text
measix-platform-core commit SHA
rikkahub_mcp commit SHA
measix-architecture commit SHA
S0.1 freeze manifest identity/hash
```

The final system/RC manifest additionally records the build/qualification/scenario evidence required by the architecture System Testing Spec.

## 4. Promotion stages

### Pull request

T0/T1/T2 for the affected repository surface, with Red/Green evidence for behavior changes. Bounded deterministic T3 runs where the repository CI requires them. No browser E2E is executed by GitHub Actions PR CI.

### Main / integration candidate

Adds required T3 lanes, migration/static-host checks and deterministic system-harness backend scenarios. GitHub Actions still stops before T4.1 browser/system E2E.

### S0.1 freeze candidate

This is an explicit candidate-verification stage outside normal GitHub Actions CI/CD. Pin the exact platform-core commit/build, then run all requirements in `measix-s0-capability-delivery-system-testing-spec.md`, including:

- real Admin browser through real Hub;
- real Hub↔Relay control/runtime paths;
- deterministic Test Client using public Client Control + Runtime APIs;
- deterministic Test Adapter for stable success/failure/no-forward evidence;
- required Managed Model/TTS/ASR/MCP profile scenarios;
- Snapshot preview/release equivalence;
- Usage/Pricing/Unknown/Partial visibility;
- required real Adapter qualification;
- Client OpenAPI/fixture/schema freeze evidence;
- no unexplained critical `CAP-*` skip.

The run may execute on a developer-controlled/local or dedicated controlled candidate environment, but it must use the exact pinned SHA and produce reproducible machine-readable evidence. A GitHub Actions `ci-gate` success is necessary baseline evidence but never substitutes for this candidate gate.

Only after this candidate passes can S0.2 Android work treat the Client contract as frozen input.

### Final S0 release candidate

Likewise, the final S0 E2E/RC verification is an explicit release-candidate run rather than part of every GitHub Actions CI/CD execution. It runs all required current-release S0 gates, including:

- valid pinned S0.1 freeze input;
- applicable Component Testing Spec MUST scenarios;
- all applicable final `SYS-*` critical scenarios;
- Android emulator/device E2E;
- Admin real-browser E2E;
- required real Adapter qualification;
- Hub/Relay restart/reconcile;
- backup/restore;
- usage spool/replay;
- target-resource/load validation;
- no unexplained critical test skip.

The architecture document is the authority for the exact Exit lists. This document only defines how the repository packages and preserves evidence.

## 5. Deterministic vs real-external lanes

Two distinct external-boundary lanes are maintained:

```text
Deterministic lane
  real MEASIX components
  deterministic Test Adapter
  no public Provider dependency

External qualification lane
  fixed Adapter version/config revision/profile
  real required upstream capability
  qualification evidence
```

A flaky public Provider must not make normal PR CI nondeterministic. Conversely, a deterministic fake cannot be used to claim real Adapter qualification.

## 6. Client contract identity

The S0.1 freeze must make the Android handoff reproducible. At minimum, it records the hash/identity of:

- `api/client/client-control.openapi.yaml`;
- the canonical fixture set used by Client/Snapshot contract tests;
- the Snapshot schema version;
- the architecture commit that defines the semantic contract;
- the platform-core commit that generated/served the contract.

After freeze, an incompatible Android-visible change creates a new architecture-approved contract/freeze candidate; it does not mutate the old evidence in place.

## 7. Build identity

Every freeze/RC binary/static build must be traceable to its source commit and build configuration. Release evidence must make it possible to determine which Hub, Relay and Admin build was tested.

## 8. Verification evidence

GitHub Actions publishes its normal T0–T3 check results. It does not execute or claim the browser/system E2E gate.

Explicit Freeze/RC verification preserves as applicable:

- machine-readable manifest;
- exact source/build identity;
- test result files;
- bounded diagnostic logs;
- browser/emulator failure artifacts when useful;
- qualification reports;
- resource/load summaries;
- started/completed timestamps and scenario result mapping.

Evidence may be retained by the controlled candidate runner and referenced from the freeze/RC manifest or release record. It does not need to be produced by GitHub Actions, but it must be durable enough to audit and reproduce the candidate.

Artifacts must not contain production credentials, real user conversations or secret plaintext.

A successful GitHub Actions workflow status without the required candidate scenario/report evidence is not sufficient for S0.1 freeze or final S0 RC.

## 9. Reproduction

A candidate should be reproducible by checking out the pinned source commits, restoring repository-controlled toolchains/lockfiles, rebuilding artifacts, and rerunning the corresponding deterministic system harness with the same declared inputs.

Moving branch heads are never used as implicit test inputs after a candidate manifest is created.

## 10. Failed candidate

When a freeze/RC scenario fails:

1. keep the failing run/artifact for diagnosis;
2. reproduce with the smallest relevant test where possible;
3. apply TDD regression workflow (Red already demonstrated by the failing scenario; add a narrower durable regression test when useful);
4. create a new candidate composition after fixes;
5. rerun affected gates and the full required candidate gate before promotion.

Do not mutate the evidence of a failed candidate to make it look successful.

## 11. Tag/version policy

Concrete repository tag/release naming is intentionally not invented by this documentation baseline. Define and document it before the first real freeze/release tag, then keep the naming rule here. Whatever naming is chosen, commit SHAs and manifest hashes remain the reproducibility identities used by system-test evidence.
