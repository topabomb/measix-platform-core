# Documentation Governance and Synchronization

> This is the documentation-governance authority for `measix-platform-core`. It defines what this repository documents, what it must only reference, and how implementation documentation stays synchronized with `topabomb/measix-architecture` without creating duplicate authorities.

## 1. Core rule

**One fact has one authority.**

`measix-architecture` answers **what the platform/product must mean, provide and prove**. `measix-platform-core` answers **what executable artifact implements that requirement and how engineers build, run, test and operate it**.

For Admin Console specifically:

```text
measix-architecture
  → product/UX requirements
  → component boundary/state/security constraints
  → testing semantics

measix-platform-core
  → concrete Vue/Quasar implementation
  → components/stores/composables
  → npm dependencies + lockfile
  → executable frontend tests/build/browser E2E
```

If a document in this repository starts re-explaining platform/product semantics already defined by architecture, replace the explanation with a short local implication plus a link/reference to the authority.

## 2. Authority matrix

| Subject | Authority | This repository may contain |
|---|---|---|
| Product/Phase/Stage/sub-stage scope | `measix-architecture` | links, implementation status |
| Admin product/UX/information architecture | `measix-architecture` | concrete UI implementation mapping only |
| Terminology / stable IDs | `measix-architecture` | generated/validated usage |
| Component responsibilities | `measix-architecture` | package/module mapping |
| Publish / activation / generation semantics | `measix-architecture` | code/test references only |
| Managed Capability profile / Client Snapshot semantics | `measix-architecture` | executable OpenAPI/fixture implementation and freeze evidence |
| Cross-component errors/state/idempotency | `measix-architecture` | executable OpenAPI shape |
| S0.1/S0.2/System required scenarios | `measix-architecture` | test code, runners, reports |
| Exact HTTP schemas | `api/*.openapi.yaml` here | generated artifacts |
| Canonical wire fixtures | `api/fixtures/` here | no duplicate private fixture sets |
| Source/package layout | source tree here | implementation documentation |
| Admin concrete frontend dependency/package choice | `console/package.json` + lockfile here | rationale in `docs/admin-console-implementation.md` |
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
- `docs/admin-console-implementation.md`: concrete Admin Vue/Quasar architecture, feature organization, UI implementation patterns and frontend dependency policy.
- `docs/development.md`: local and GitHub-only development procedures.
- `docs/api-contracts.md`: OpenAPI/fixture/codegen and client-contract freeze workflow.
- `docs/testing.md`: executable test organization, S0.1/S0.2 gates, CI and evidence.
- `docs/tdd.md`: Red/Green/Refactor workflow.
- `docs/database-migrations.md`: schema/migration execution.
- `docs/operations.md`: runtime/deployment/backup/restore operations.
- `docs/release.md`: S0.1 freeze-candidate and final S0 RC composition/verification.

Do not create local copies named `control-protocol.md`, `s0-architecture.md`, `s0.1-capability-contract.md`, `admin-console-product-requirements.md`, `android-integration-contract.md`, `publish-flow.md`, `identifier-contract.md`, `control-hub-architecture.md`, `runtime-relay-architecture.md` or equivalent. Those would duplicate architecture authority.

## 4. Synchronization workflows

### A. Semantic/product architecture change

Examples: new stable ID, changed Publish meaning, changed admission rule, changed Managed Capability field semantics, changed Admin required workflow/information architecture, new Problem semantic, changed S0.1/S0.2 gate or required S0 scenario.

Order:

```text
1. change measix-architecture authority
2. merge/freeze the architecture/product decision
3. update OpenAPI/fixtures/tests here if affected
4. update concrete implementation here
5. update Android repository when affected
```

The implementation PR must reference the architecture commit or PR that authorized the change.

### B. Admin concrete implementation change

The following normally stay in this repository and do **not** require an architecture edit:

```text
Vue component/page/store/composable organization
normal Quasar UI composition
chart/topology/date/helper library choice
frontend dependency version upgrades
internal UI refactor
browser performance/bundle optimization
```

They require an architecture change only when they alter product requirements, security boundary, wire meaning, persistent-state semantics, required workflow or required testing behavior.

There is no architecture-maintained npm whitelist. A mature dependency may be added when it solves a real UI/engineering problem and satisfies `docs/admin-console-implementation.md` dependency rules.

### C. S0.1 Client Contract Freeze

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

### D. Non-semantic executable-contract completion

If architecture already fixes the meaning and implementation only completes exact schema details without changing that meaning:

```text
architecture semantic baseline
  → OpenAPI change here
  → fixture/codegen drift checks
  → consumers/tests
```

Do not copy the completed schema back into architecture Markdown. If the schema completion exposes ambiguity, stop and use workflow A.

### E. Regression discovered in implementation

Every meaningful regression fix gets a regression test here. If the bug reveals a missing/incorrect architecture/product requirement or a critical scenario worth long-term tracking, update the relevant Architecture/Product/Testing document first.

## 5. Pull-request synchronization declaration

Every non-trivial PR declares one of:

```text
Architecture impact: none
Architecture impact: implementation of existing requirement <document/section or scenario ID>
Architecture impact: requires architecture change <architecture PR/commit>
```

A PR must not say `none` when it changes product requirements, wire meaning, state semantics, IDs, security invariants or required test behavior.

When the work spans an S0 delivery sub-stage, also state whether it is:

```text
S0.1 implementation / freeze work
S0.2 Android-handoff support
final S0 RC/Exit work
```

## 6. Cross-repository versioning

Normal development follows the latest merged architecture baseline. Reproducible integration/freeze/RC evidence pins concrete commits instead of relying on moving branch heads.

For S0.1 Admin implementation after 2026-08-20, use architecture commit `6de9bfb794e60e9bb6c62501263cc1518e4f5ee3` or a later merged authority as the baseline for Product/UX Requirements.

For the S0.1 freeze, the manifest pins at least the architecture commit, platform-core commit, Client OpenAPI hash, canonical fixture hash, Snapshot schema version, Admin build identity and required qualification/scenario evidence. It does **not** require an Android commit because S0.1 is intentionally pre-Android.

For final S0 RC, the manifest additionally pins the commits required by the final S0 System Testing Spec, including `rikkahub_mcp`.

A source commit never embeds a copied architecture document merely to pin it. Pin by repository + commit SHA/link.

## 7. Updating implementation documents

Implementation documents are updated when the executable workflow changes:

- current delivery status/gap changes → `docs/s0-execution-progress.md`;
- Admin concrete UI structure/dependency/implementation pattern changes → `docs/admin-console-implementation.md`;
- new build or codegen process → `docs/development.md` / `docs/api-contracts.md`;
- Client OpenAPI freeze/handoff change → `docs/api-contracts.md` / `docs/release.md`;
- new test lane or runner → `docs/testing.md`;
- TDD/PR process change → `docs/tdd.md` / `CONTRIBUTING.md`;
- migration process change → `docs/database-migrations.md`;
- deployment/config/backup change → `docs/operations.md`;
- S0.1 freeze or final RC composition/evidence change → `docs/release.md`.

Do not edit an architecture document just because a local command, Vue file, npm package or package version changed.

## 8. Review checklist for documentation changes

Before merging a documentation change, verify:

1. the statement belongs in this repository;
2. no second product/architecture authority was created;
3. architecture concepts are linked/referenced, not redefined;
4. commands describe artifacts that actually exist, or are explicitly marked as implementation targets;
5. exact paths/names match the current tree;
6. testing documentation points to architecture requirements/scenario IDs rather than redefining them;
7. operational documents contain no production secrets;
8. cross-repository changes identify their corresponding commits/PRs;
9. `docs/s0-execution-progress.md` does not claim a delivery gate is complete before its required evidence exists;
10. pre-freeze and frozen Client contracts are not described interchangeably;
11. frontend dependencies listed as “current” actually exist in `package.json`, while planned dependencies are explicitly marked as planned/conditional.
