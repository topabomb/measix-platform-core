# Documentation Governance and Synchronization

> This is the documentation-governance authority for `measix-platform-core`. It defines what this repository documents, what it must only reference, and how implementation documentation stays synchronized with `topabomb/measix-architecture` without creating duplicate authorities.

## 1. Core rule

**One fact has one authority.**

`measix-architecture` answers **what the platform must mean and prove**. `measix-platform-core` answers **what executable artifact implements that requirement and how engineers build, run, test and operate it**.

If a document in this repository starts re-explaining platform behavior already defined by architecture, replace the explanation with a short local implication plus a link to the authority.

## 2. Authority matrix

| Subject | Authority | This repository may contain |
|---|---|---|
| Product/Phase/Stage/sub-stage scope | `measix-architecture` | links, implementation status |
| Terminology / stable IDs | `measix-architecture` | generated/validated usage |
| Component responsibilities | `measix-architecture` | package/module mapping |
| Publish / activation / generation semantics | `measix-architecture` | code/test references only |
| Managed Capability profile / Client Snapshot semantics | `measix-architecture` | executable OpenAPI/fixture implementation and freeze evidence |
| Cross-component errors/state/idempotency | `measix-architecture` | executable OpenAPI shape |
| S0.1/S0.2/System required scenarios | `measix-architecture` | test code, runners, reports |
| Exact HTTP schemas | `api/*.openapi.yaml` here | generated artifacts |
| Canonical wire fixtures | `api/fixtures/` here | no duplicate private fixture sets |
| Source/package layout | source tree here | implementation documentation |
| Ent schema / migration SQL | source here | migration procedure |
| Environment/config names | source + `docs/operations.md` here | complete operational reference |
| Build/codegen/test commands | repository tooling here | README/development/testing docs |
| CI workflow and required checks | `.github/workflows/` here | process documentation |
| S0.1 freeze / final S0 release evidence | CI artifacts/manifests here | release procedure |

## 3. Document set in this repository

Root documents:

- `README.md`: project entry point, current delivery-stage summary and navigation only.
- `ARCHITECTURE.md`: implementation boundaries/dependency direction, not product architecture.
- `CONTRIBUTING.md`: contribution and PR rules.
- `AGENTS.md`: coding-agent execution rules.

Engineering documents:

- `docs/documentation-governance.md`: this authority/synchronization policy.
- `docs/s0-execution-progress.md`: current implementation plan/status/evidence against the merged architecture baseline; it may record gaps but must not redefine their semantics.
- `docs/development.md`: local and GitHub-only development procedures.
- `docs/api-contracts.md`: OpenAPI/fixture/codegen and client-contract freeze workflow.
- `docs/testing.md`: executable test organization, S0.1/S0.2 gates, CI and evidence.
- `docs/tdd.md`: Red/Green/Refactor workflow.
- `docs/database-migrations.md`: schema/migration execution.
- `docs/operations.md`: runtime/deployment/backup/restore operations.
- `docs/release.md`: S0.1 freeze-candidate and final S0 RC composition/verification.

Do not create local copies named `control-protocol.md`, `s0-architecture.md`, `s0.1-capability-contract.md`, `android-integration-contract.md`, `publish-flow.md`, `identifier-contract.md`, `control-hub-architecture.md`, `runtime-relay-architecture.md` or equivalent. Those would duplicate architecture authority.

## 4. Synchronization workflows

### A. Semantic architecture change

Examples: new stable ID, changed Publish meaning, changed admission rule, changed Managed Capability field semantics, new Problem semantic, changed S0.1/S0.2 gate or required S0 scenario.

Order:

```text
1. change measix-architecture authority
2. merge/freeze the architecture decision
3. update OpenAPI/fixtures/tests here
4. update implementation here
5. update Android repository when affected
```

The implementation PR must reference the architecture commit or PR that authorized the change.

### B. S0.1 Client Contract Freeze

S0.1 exists specifically so server-side product capability can be completed and verified before Android integration. The architecture repository owns the freeze criteria; this repository owns the executable artifacts and evidence.

The local synchronization flow is:

```text
merged S0.1 architecture baseline
→ align Client/Admin/Internal OpenAPI
→ canonical fixtures + generated artifacts
→ Admin/Hub/Relay/Adapter/Usage implementation
→ deterministic S0.1 system gate
→ required real Adapter qualification
→ produce S0.1 freeze manifest
→ pin Client OpenAPI + fixture hashes + snapshot schema
→ hand off to S0.2 Android implementation
```

Before that manifest exists, `client-control.openapi.yaml` and Snapshot v1 are **pre-freeze**. After the freeze, incompatible changes do not silently rewrite the frozen contract; they follow the architecture-defined compatibility/versioning process and require explicit downstream synchronization.

### C. Non-semantic executable-contract completion

If architecture already fixes the meaning and implementation only completes exact schema details without changing that meaning:

```text
architecture semantic baseline
  → OpenAPI change here
  → fixture/codegen drift checks
  → consumers/tests
```

Do not copy the completed schema back into architecture Markdown. If the schema completion exposes ambiguity, stop and use workflow A.

### D. Pure implementation change

Package refactors, internal helper changes, DB indexes, test harness improvements, CI caching, build tooling and UI decomposition remain entirely in this repository unless they violate or require changing an architectural invariant.

### E. Regression discovered in implementation

Every meaningful regression fix gets a regression test here. If the bug reveals a missing/incorrect architecture requirement or a critical scenario worth long-term tracking, update the relevant Architecture/Testing Spec and assign/reference the stable scenario ID there before treating this repository as the semantic source.

## 5. Pull-request synchronization declaration

Every non-trivial PR declares one of:

```text
Architecture impact: none
Architecture impact: implementation of existing requirement <document/section or scenario ID>
Architecture impact: requires architecture change <architecture PR/commit>
```

A PR must not say `none` when it changes wire meaning, state semantics, IDs, security invariants or required test behavior.

When the work spans an S0 delivery sub-stage, also state whether it is:

```text
S0.1 implementation / freeze work
S0.2 Android-handoff support
final S0 RC/Exit work
```

## 6. Cross-repository versioning

Normal development follows the latest merged architecture baseline. Reproducible integration/freeze/RC evidence pins concrete commits instead of relying on moving branch heads.

For the S0.1 freeze, the manifest pins at least the architecture commit, platform-core commit, Client OpenAPI hash, canonical fixture hash, Snapshot schema version, Admin build identity and required qualification/scenario evidence. It does **not** require an Android commit because S0.1 is intentionally pre-Android.

For final S0 RC, the manifest additionally pins the commits required by the final S0 System Testing Spec, including `rikkahub_mcp`.

A source commit never embeds a copied architecture document merely to pin it. Pin by repository + commit SHA/link.

## 7. Updating implementation documents

Implementation documents are updated when the executable workflow changes:

- current delivery status/gap changes → `docs/s0-execution-progress.md`;
- new build or codegen process → `docs/development.md` / `docs/api-contracts.md`;
- Client OpenAPI freeze/handoff change → `docs/api-contracts.md` / `docs/release.md`;
- new test lane or runner → `docs/testing.md`;
- TDD/PR process change → `docs/tdd.md` / `CONTRIBUTING.md`;
- migration process change → `docs/database-migrations.md`;
- deployment/config/backup change → `docs/operations.md`;
- S0.1 freeze or final RC composition/evidence change → `docs/release.md`.

Do not edit an architecture document just because a local command or package path changed.

## 8. Review checklist for documentation changes

Before merging a documentation change, verify:

1. the statement belongs in this repository;
2. no second authority was created;
3. architecture concepts are linked, not restated in full;
4. commands describe artifacts that actually exist, or are explicitly marked as implementation targets;
5. exact paths/names match the current tree;
6. testing documentation points to architecture scenario IDs rather than redefining them;
7. operational documents contain no production secrets;
8. cross-repository changes identify their corresponding commits/PRs;
9. `docs/s0-execution-progress.md` does not claim a delivery gate is complete before its required evidence exists;
10. pre-freeze and frozen Client contracts are not described interchangeably.
