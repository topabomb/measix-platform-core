# Release, Versioned S0 Freezes and Final Release-Candidate Verification

This document defines how implementation candidates are composed and proven reproducibly. The architecture S0.1/S0.2/S0.3/S0.4 Testing Specs and final S0 System Testing Spec remain authoritative for what must pass.

## 1. Release principle

A candidate is a fixed, reproducible composition of source commits, generated contracts, builds and test evidence — never an implicit moving branch head.

```text
S0.1 Client Contract Freeze Candidate
  → pre-Android server-side product closure

S0.2 Realm/Experience Freeze Candidate
  → Snapshot v2 and product foundation

S0.3 Gateway Freeze Candidate
  → Snapshot v3 + three-daemon Gateway server closure

S0.4 Android Integration Candidate
  → real Android full managed runtime profile

Final S0 Release Candidate
  → pinned S0.1–S0.4 composition + final S0 Exit gate
```

GitHub Actions CI/CD provides the fast deterministic T0–T3 baseline. Browser T4.1 and real external Adapter qualification are explicit promotion gates on an exact candidate SHA.

## 2. S0.1 freeze candidate

S0.1 is intentionally pre-Android. A candidate may exist while verification is incomplete; only an accepted Freeze requires the full architecture-defined C6/C7 requirements to be Green.

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

It does **not** require `androidCommit`; later sub-stages consume this pinned baseline.

The exact serialized manifest schema is implemented by the candidate/system harness. Markdown documents must not create a competing schema or use different field names such as a generic `adapterQualificationRef` when architecture requires `realAdapterQualificationRef`.

### Candidate draft, accepted Freeze and historical evidence

`docs/s0-freeze-manifest.json` is generated evidence, not a planning/config file. Current tooling uses two phases: `freeze-manifest.mjs` writes a candidate with CAP-C7-002=NOT_EXECUTED; `replay-freeze.mjs` can update it after replay. Only the fully proven composition is an accepted Freeze. A candidate draft or historical record must never be presented as current acceptance.

Both scripts can write the tracked manifest path. Run candidate generation in an isolated candidate checkout, preserve existing historical records/artifacts and do not overwrite a known baseline during ordinary development. A future packaging change should separate candidate output from immutable accepted evidence. If any pinned source/build/contract identity changes, rerun the required gate for the new composition.

The retained manifest declares architecture `cc60f8f540d309f2b73228094c8b9cd1b0b0a60f`, core `a6075bc0afd78fa86d77e1a520f838c954c9adfa`, Snapshot v1. The 2026-08-31 audit did not replay/fully validate its external artifact chain; neither its presence nor declared PASS proves current HEAD or later stages.

### Current tooling limitations

See [the source-backed audit](architecture-alignment-audit.md), A14–A15. Collection Make recipes have working-directory/error-propagation defects; the writer hardcodes schemaVersion=1 while the current compiler emits v2; validation does not check the complete input/provenance chain. Replay creates a fresh runtime but does not establish an independent clean source checkout/rebuild of all pinned assets. Real qualification's top-level VERIFIED does not imply every required capability profile was exercised. Do not present `make freeze-gate`, `--validate` or `clean-replay` as complete proven acceptance until these gaps are repaired and tested.

## 3. S0.2/S0.3/S0.4 candidates

Each later sub-stage pins its own architecture/core/consumer/build/contract/scenario identities and consumes the previous valid freeze; a historical earlier manifest cannot prove a later candidate.

S0.3 specifically requires a real `enterprise-tool-gateway` production binary/build identity, Gateway Control OpenAPI/hash, Snapshot v3 and canonical surface/catalog fixtures, real Hub/Gateway/Relay + downstream MCP + Test Client traffic, production Admin browser evidence, and executable production supervision/graceful lifecycle/structured-log collection/redaction evidence. The current repository does not yet provide these artifacts.

S0.4 adds pinned real Android implementation/device evidence against the S0.3 baseline. Exact composition fields remain owned by architecture Testing Specs and executable harness schemas.

## 4. Final S0 release candidate

Final S0 RC consumes specific valid S0.1/S0.2/S0.3/S0.4 baselines and their real server, Portal and Android implementations.

At minimum the composition fixes:

```text
measix-platform-core commit SHA
rikkahub_mcp commit SHA
measix-architecture commit SHA
S0.1 freeze manifest identity/hash
```

The final manifest additionally records the build/qualification/scenario evidence required by `measix-s0-system-testing-spec.md`.

## 5. Promotion stages

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

Final S0 verification composes every pinned sub-stage baseline and all applicable current-release gates, including:

- valid pinned S0.1 freeze;
- applicable Component Testing Spec MUST scenarios;
- applicable final `ERX-*` / `ETG-*` / `AND-*` / `SYS-*` scenarios;
- Android emulator/device E2E;
- Admin real-browser E2E;
- real Hub/Gateway/Relay service topology and Gateway downstream MCP;
- required real Adapter qualification;
- Hub/Gateway/Relay restart/reconcile and production supervisor lifecycle;
- structured log collection/correlation/redaction proof;
- backup/restore;
- usage spool/replay;
- target-resource/load validation;
- no unexplained critical skip.

Architecture is authoritative for exact Exit requirements.

## 6. Deterministic vs real-external lanes

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

## 7. Client contract identity

The S0.1 freeze makes the Android handoff reproducible by pinning at least:

- `api/client/client-control.openapi.yaml` identity/hash;
- canonical fixture set identity/hash;
- Snapshot schema version;
- architecture commit defining semantic contract;
- platform-core commit/build serving the contract.

After freeze, an incompatible Android-visible change creates a new architecture-approved contract/freeze candidate; old evidence is immutable.

## 8. Build identity and evidence

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

## 9. Failed candidate

When a freeze/RC scenario fails:

1. retain failing evidence for diagnosis;
2. reproduce with the smallest relevant durable test where possible;
3. apply TDD regression workflow;
4. create a new candidate composition after fixes;
5. rerun affected gates and the full required candidate gate before promotion.

Do not mutate failed evidence to make it appear successful.

## 10. Reproduction and versioning

A candidate must be reproducible by checking out pinned commits, restoring repository-controlled toolchains/lockfiles, rebuilding artifacts and rerunning the corresponding deterministic/system verification with declared inputs.

Concrete tag/release naming may be defined before the first real freeze/release tag. Commit SHAs and manifest hashes remain the primary reproducibility identities.
