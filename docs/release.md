# Release, Versioned S0 Freezes and Final Release-Candidate Verification

This document defines how implementation candidates are composed and proven reproducibly. The architecture S0.1/S0.2/S0.3/S0.4 Testing Specs and final S0 System Testing Spec remain authoritative for what must pass.

## 1. Release principle

A candidate is a fixed, reproducible composition of source commits, generated contracts, builds and test evidence — never an implicit moving branch head.

For the S0.2 internal Preview, `node scripts/build-preview-release.mjs <version>` is the production artifact owner. It requires clean pinned Core, Portal, architecture and Android worktrees, verifies the `S0.2/v5-starter-preview` protocol baseline and regenerated artifacts, builds Admin/Portal assets, cross-compiles static Linux ARM64 binaries with a non-`dev` build identity, and emits `release.json` plus `SHA256SUMS`. When the active Android worktree contains unrelated local work, set `MEASIX_RELEASE_ANDROID_ROOT` to a clean detached worktree at the exact Android commit being recorded; the builder still rejects a dirty override. The target runbook is [S0.2 Preview deployment](s02-preview-deployment.md). This package is a Preview delivery vehicle, not proof of later S0.3/S0.4/final gates.

### S0.2 sealed Preview composition

The project designates `0.2.0-preview.22` as its S0.2 Preview baseline. The generated release package pins Architecture `a52a630c75f5a857ebf72834d6770cae1cef14b8`, Core `632ade34a14e64b266d86da9e4b034e5005f0713`, Portal `7e3f5fb90955a2a44bea925e4f44e62267aab9bd`, and Android `1914e4a894979bb5a2fc892a019b9342ff60a28f`. The Linux ARM64 archive SHA-256 is `ca7d480dccc6311923bcc024fd67ef7a42a82dca256567f25bb7b1212dfa2c28`. Its `release.json` and `SHA256SUMS` are under `.artifacts/releases/measix-core-0.2.0-preview.22-linux-arm64/`; preserve them with the archive when distributing this fixed composition.

The user confirms that real-device S0.2 integration was performed. The release manifest proves source, protocol, build and archive identity; it does not enumerate `ERX-*` results or identify a device/APK run. Keep the real-device and browser execution records beside this fixed release when available, with their actual device/build identities and scenario outcomes. Do not manufacture PASS rows from the project decision or relabel the CAP-only `freeze-manifest.mjs` output as an ERX manifest. The current evidence-index boundary is summarized in [S0 status](s0-execution-progress.md).

```text
S0.1 Client Contract Freeze Candidate
  → pre-Android server-side product closure

S0.2 Realm/Experience Freeze Candidate
  → Snapshot v4 and product foundation with five user-configuration policy flags

S0.3 Gateway Freeze Candidate
  → Snapshot v6 + three-daemon Gateway server closure

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

### Candidate draft and accepted Freeze

Candidates default to `.artifacts/s0-freeze-candidate.json`; writes are exclusive. Use a new output path for a new candidate. A draft with CAP-C7-002=NOT_EXECUTED is never a Freeze.

Evidence for the exact commit being frozen must already exist. The writer rejects a missing or stale artifact (`missing artifact/meta`, `source mismatch`, `artifact hash mismatch`), because every `*.meta.json` records the platform-core and architecture commits, tree cleanliness, exit code and artifact hash. Collect all eight on a clean tree at the frozen commit:

```text
make collect-artifacts                                   # backend/system/console test JSON
make collect-candidate                                   # candidate lane JSON, required by the CAP scenarios
make collect-static-contract                             # gofmt, go vet, codegen drift
make collect-baseline                                    # resource baseline, must be GREEN
make collect-adapter-qualification ENDPOINT=... KEY=...   # all four profiles VERIFIED
make s01-browser-candidate                               # e2e-playwright.json
```

Use the following sequence on a clean pinned composition:

```text
node scripts/freeze-manifest.mjs --validate --candidate --manifest <candidate.json>
node scripts/replay-freeze.mjs --manifest <candidate.json>
node scripts/freeze-manifest.mjs --finalize --manifest <candidate.json> --output <new-final.json>
node scripts/freeze-manifest.mjs --validate --manifest <new-final.json>
```

Replay clones pinned Core and architecture commits into independent sibling checkouts, excluding worktree edits, local secrets, dependencies and build artifacts. It installs locked dependencies, regenerates contracts, checks format/types, rebuilds the production SPA and compares all source/contract/fixture/build pins. Then it runs contract, vet, backend, smoke, system (adapter/client), candidate, console and the complete browser harness. Its workspace lives under `.artifacts/replay/` because finalization reads the per-stage logs back. Regeneration or tests that dirty the checkout fail.

Full logs remain in the reported independent workspace; preserve that workspace with the evidence. `.artifacts/replay-artifact.json` is written exclusively, including failures, and binds original candidate bytes, rebuilt facts and ordered command results. Finalization validates the report and log hashes, updates CAP-C7-002 in a new manifest, and leaves the candidate untouched. Existing output is never overwritten. There is no runtime-only shortcut. Changed source/build/contract requires a new composition and gate run.


### Current tooling limitations

The writer validates current source/architecture cleanliness and identity, production build, four OpenAPI hashes, fixture/schema/Adapter pins, complete required scenario results, and every artifact plus metadata hash/exit/source. Qualification requires all four profiles in one run, each with observed adapter version, upstream/config revision, transport and forwarded usage evidence; unknown identity or an unexecuted profile fails. Partial diagnostic runs cannot be merged into qualification. Declared NONE/LEVEL_0 is not semantic Usage/header-echo qualification.

This CAP manifest compiler verifies the resource baseline of current Snapshot v4 but does not produce S0.2 ERX/consumer evidence. The clean-source runner is implemented; passing runner fixture tests is not an executed candidate replay. Candidate promotion records each required gate explicitly. The fixed S0.2 Preview identity and current evidence index are in [S0 status](s0-execution-progress.md).

## 3. S0.2 evidence and later-stage candidates

The S0.2 Preview composition above is fixed; its ERX results and consumer evidence must be traceable to that exact composition. S0.3 and S0.4 each pin their own architecture/core/consumer/build/contract/scenario identities and consume the applicable earlier baseline; an earlier manifest cannot prove a later candidate.

S0.3 specifically requires a real `enterprise-tool-gateway` production binary/build identity, Gateway Control OpenAPI/hash, Snapshot v6 and canonical surface/catalog fixtures, real Hub/Gateway/Relay + downstream MCP + Test Client traffic, production Admin browser evidence, and executable production supervision/graceful lifecycle/structured-log collection/redaction evidence. The current repository does not yet provide these artifacts. Current supported v4/v5 data preservation follows Control Protocol §10.10.1; future Gateway requires its own candidate and evidence.

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

Adds required T3 lanes, current-schema/static-host checks and deterministic backend/system-harness scenarios. GitHub Actions still does not imply S0.1 C6/C7 completion.

### S0.1 freeze candidate

Pin the exact platform-core SHA/build and run all requirements in `measix-s0-capability-delivery-system-testing-spec.md`, including:

- real Admin browser through real Hub;
- real Hub↔Relay control/runtime paths;
- deterministic Test Client using public Client Control + Runtime APIs;
- deterministic Test Adapter for stable success/failure/no-forward evidence;
- required Model/Image Generation/TTS/ASR/MCP profile scenarios;
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
