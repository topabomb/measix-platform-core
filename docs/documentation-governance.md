# Documentation Governance and Synchronization

> This is the documentation-governance authority for `measix-platform-core`. It defines what this repository documents, what it must only reference, and how implementation documentation stays synchronized with `topabomb/measix-architecture` without creating duplicate authorities.

## 1. Core rule

**One fact has one authority.**

`measix-architecture` answers **what the platform must mean and prove**. `measix-platform-core` answers **what executable artifact implements that requirement and how engineers build, run, test and operate it**.

If a document in this repository starts re-explaining platform behavior already defined by architecture, replace the explanation with a short local implication plus a link to the authority.

## 2. Authority matrix

| Subject | Authority | This repository may contain |
|---|---|---|
| Product/Phase/Stage scope | `measix-architecture` | links, implementation status |
| Terminology / stable IDs | `measix-architecture` | generated/validated usage |
| Component responsibilities | `measix-architecture` | package/module mapping |
| Publish / activation / generation semantics | `measix-architecture` | code/test references only |
| Cross-component errors/state/idempotency | `measix-architecture` | executable OpenAPI shape |
| Required Component/System scenarios | `measix-architecture` | test code, runners, reports |
| Exact HTTP schemas | `api/*.openapi.yaml` here | generated artifacts |
| Canonical wire fixtures | `api/fixtures/` here | no duplicate private fixture sets |
| Source/package layout | source tree here | implementation documentation |
| Ent schema / migration SQL | source here | migration procedure |
| Environment/config names | source + `docs/operations.md` here | complete operational reference |
| Build/codegen/test commands | repository tooling here | README/development/testing docs |
| CI workflow and required checks | `.github/workflows/` here | process documentation |
| Release test evidence | CI artifacts/manifests here | release procedure |

## 3. Document set in this repository

Root documents:

- `README.md`: project entry point and navigation only.
- `ARCHITECTURE.md`: implementation boundaries/dependency direction, not product architecture.
- `CONTRIBUTING.md`: contribution and PR rules.
- `AGENTS.md`: coding-agent execution rules.

Engineering documents:

- `docs/documentation-governance.md`: this authority/synchronization policy.
- `docs/development.md`: local and GitHub-only development procedures.
- `docs/api-contracts.md`: OpenAPI/fixture/codegen workflow.
- `docs/testing.md`: executable test organization, CI and evidence.
- `docs/tdd.md`: Red/Green/Refactor workflow.
- `docs/database-migrations.md`: schema/migration execution.
- `docs/operations.md`: runtime/deployment/backup/restore operations.
- `docs/release.md`: reproducible RC composition and release verification.

Do not create local copies named `control-protocol.md`, `s0-architecture.md`, `publish-flow.md`, `identifier-contract.md`, `control-hub-architecture.md`, `runtime-relay-architecture.md` or equivalent. Those would duplicate architecture authority.

## 4. Synchronization workflows

### A. Semantic architecture change

Examples: new stable ID, changed Publish meaning, changed admission rule, new Problem semantic, changed required S0 scenario.

Order:

```text
1. change measix-architecture authority
2. merge/freeze the architecture decision
3. update OpenAPI/fixtures/tests here
4. update implementation here
5. update Android repository when affected
```

The implementation PR must reference the architecture commit or PR that authorized the change.

### B. Non-semantic executable-contract completion

If architecture already fixes the meaning and implementation only completes exact schema details without changing that meaning:

```text
architecture semantic baseline
  → OpenAPI change here
  → fixture/codegen drift checks
  → consumers/tests
```

Do not copy the completed schema back into architecture Markdown. If the schema completion exposes ambiguity, stop and use workflow A.

### C. Pure implementation change

Package refactors, internal helper changes, DB indexes, test harness improvements, CI caching, build tooling and UI decomposition remain entirely in this repository unless they violate or require changing an architectural invariant.

### D. Regression discovered in implementation

Every meaningful regression fix gets a regression test here. If the bug reveals a missing/incorrect architecture requirement or a critical scenario worth long-term tracking, update the relevant Architecture/Testing Spec and assign/reference the stable scenario ID there before treating this repository as the semantic source.

## 5. Pull-request synchronization declaration

Every non-trivial PR declares one of:

```text
Architecture impact: none
Architecture impact: implementation of existing requirement <document/section or scenario ID>
Architecture impact: requires architecture change <architecture PR/commit>
```

A PR must not say `none` when it changes wire meaning, state semantics, IDs, security invariants or required test behavior.

## 6. Cross-repository versioning

Normal development follows the latest merged architecture baseline. Reproducible integration/RC evidence pins concrete commits instead of relying on moving branch heads.

For S0 RC, the test manifest must at minimum pin the commits required by the S0 System Testing Spec (`measix-platform-core` and `rikkahub_mcp`). It should also record the architecture baseline commit used to interpret those binaries and tests.

A source commit never embeds a copied architecture document merely to pin it. Pin by repository + commit SHA/link.

## 7. Updating implementation documents

Implementation documents are updated when the executable workflow changes:

- new build or codegen process → `docs/development.md` / `docs/api-contracts.md`;
- new test lane or runner → `docs/testing.md`;
- TDD/PR process change → `docs/tdd.md` / `CONTRIBUTING.md`;
- migration process change → `docs/database-migrations.md`;
- deployment/config/backup change → `docs/operations.md`;
- RC composition/evidence change → `docs/release.md`.

Do not edit an architecture document just because a local command or package path changed.

## 8. Review checklist for documentation changes

Before merging a documentation change, verify:

1. the statement belongs in this repository;
2. no second authority was created;
3. architecture concepts are linked, not restated in full;
4. commands describe artifacts that actually exist, or are explicitly marked as I0 target contracts;
5. exact paths/names match the current tree;
6. testing documentation points to architecture scenario IDs rather than redefining them;
7. operational documents contain no production secrets;
8. cross-repository changes identify their corresponding commits/PRs.
