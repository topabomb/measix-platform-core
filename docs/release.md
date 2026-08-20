# Release, S0.1 Freeze and Final S0 Release-Candidate Verification

This document defines how implementation candidates are composed and proven reproducibly. The architecture S0.1 Capability Delivery System Testing Spec and final S0 System Testing Spec remain authoritative for what must pass.

## 1. Release principle

A candidate is a fixed, reproducible composition of source commits, generated contracts, builds and test evidence — never an implicit moving branch head.

```text
S0.1 Client Contract Freeze Candidate
  → pre-Android server-side product closure

Final S0 Release Candidate
  → pinned S0.1 contract + real Android S0.2 integration + final S0 Exit gate
```

GitHub Actions CI/CD provides the fast deterministic T0–T3 baseline. Browser T4.1 and real external Adapter qualification are explicit promotion gates on an exact candidate SHA.

## 2. S0.1 freeze candidate

S0.1 is intentionally pre-Android. A freeze candidate can exist only after the architecture-defined C6/C7 requirements are Green.

The machine-readable manifest must include the identities required by the current architecture System Testing Spec, including:

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

It does **not** require `androidCommit`; Android S0.2 starts only after this freeze.

The exact serialized manifest schema is implemented by the candidate/system harness. Markdown documents must not create a competing schema or use different field names such as a generic `adapterQualificationRef` when architecture requires `realAdapterQualificationRef`.

### No placeholder manifest

`docs/s0-freeze-manifest.json` is generated evidence, not a planning/config file. It must not exist as a stale/preliminary placeholder. Generate it only when the exact candidate has passed all required C6/C7 scenarios and real Adapter qualification. If the candidate SHA changes, rerun the required gate and generate new evidence.

## 3. Final S0 release candidate

Final S0 RC consumes a specific valid S0.1 freeze and adds the real Android implementation.

At minimum the composition fixes:

```text
measix-platform-core commit SHA
rikkahub_mcp commit SHA
measix-architecture commit SHA
S0.1 freeze manifest identity/hash
```

The final manifest additionally records the build/qualification/scenario evidence required by `measix-s0-system-testing-spec.md`.

## 4. Promotion stages

### Pull request

Run affected T0/T1/T2 plus bounded deterministic T3 where required. Behavior changes retain Red/Green evidence. Default PR CI does not run full browser T4.1.

### Main / integration candidate

Adds required T3 lanes, migration/static-host checks and deterministic backend/system-harness scenarios. GitHub Actions still does not imply S0.1 C6/C7 completion.

### S0.1 freeze candidate

Pin the exact platform-core SHA/build and run all requirements in `measix-s0-capability-delivery-system-testing-spec.md`, including:

- real Admin browser through real Hub;
- real Hub↔Relay control/runtime paths;
- deterministic Test Client using public Client Control + Runtime APIs;
- deterministic Test Adapter for stable success/failure/no-forward evidence;
- required Model/TTS/ASR/MCP profile scenarios;
- Snapshot Preview/Release equivalence;
- Usage/Pricing/UNKNOWN/PARTIAL visibility;
- recovery/security/generation scenarios;
- required real Adapter qualification;
- Client OpenAPI/fixture/schema freeze evidence;
- no unexplained critical `CAP-*` skip.

The run may execute in a controlled local/dedicated environment, but it must use the exact pinned SHA and produce durable machine-readable evidence. A Green GitHub `ci-gate` is necessary baseline evidence but never substitutes for this gate.

Only after this candidate passes may S0.2 treat the Client contract as frozen input.

### Final S0 release candidate

Final S0 verification extends the frozen S0.1 composition with Android and all applicable current-release gates, including:

- valid pinned S0.1 freeze;
- applicable Component Testing Spec MUST scenarios;
- applicable final `AND-*` / `SYS-*` scenarios;
- Android emulator/device E2E;
- Admin real-browser E2E;
- required real Adapter qualification;
- Hub/Relay restart/reconcile;
- backup/restore;
- usage spool/replay;
- target-resource/load validation;
- no unexplained critical skip.

Architecture is authoritative for exact Exit requirements.

## 5. Deterministic vs real-external lanes

```text
Deterministic lane
  real MEASIX components
  deterministic Test Adapter
  no public Provider dependency

External qualification lane
  fixed Adapter version/config/profile
  real required upstream capability
  qualification evidence
```

A flaky public Provider must not make normal PR CI nondeterministic. Conversely, deterministic Adapter evidence cannot be reported as real Adapter qualification.

## 6. Client contract identity

The S0.1 freeze makes the Android handoff reproducible by pinning at least:

- `api/client/client-control.openapi.yaml` identity/hash;
- canonical fixture set identity/hash;
- Snapshot schema version;
- architecture commit defining semantic contract;
- platform-core commit/build serving the contract.

After freeze, an incompatible Android-visible change creates a new architecture-approved contract/freeze candidate; old evidence is immutable.

## 7. Build identity and evidence

Every freeze/RC binary/static build must be traceable to source commit and build configuration. Candidate evidence preserves as applicable:

- machine-readable manifest;
- exact source/build identities;
- scenario result files;
- bounded safe diagnostics;
- browser/emulator failure artifacts;
- deterministic Adapter version;
- real Adapter qualification reference;
- started/completed timestamps.

Artifacts must not contain production credentials, real user conversations or Secret plaintext.

## 8. Failed candidate

When a freeze/RC scenario fails:

1. retain failing evidence for diagnosis;
2. reproduce with the smallest relevant durable test where possible;
3. apply TDD regression workflow;
4. create a new candidate composition after fixes;
5. rerun affected gates and the full required candidate gate before promotion.

Do not mutate failed evidence to make it appear successful.

## 9. Reproduction and versioning

A candidate must be reproducible by checking out pinned commits, restoring repository-controlled toolchains/lockfiles, rebuilding artifacts and rerunning the corresponding deterministic/system verification with declared inputs.

Concrete tag/release naming may be defined before the first real freeze/release tag. Commit SHAs and manifest hashes remain the primary reproducibility identities.
