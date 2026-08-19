# Documentation Governance

`measix-architecture` owns **what MEASIX must mean and prove**. This repository owns **the executable artifacts and engineering implementation**.

## Ownership

| Subject | Authority |
|---|---|
| Phase/Stage/sub-stage scope | `measix-architecture` |
| Product/UX requirements | `measix-architecture` |
| Terminology / stable IDs | `measix-architecture` |
| Cross-component state/wire/error/security semantics | `measix-architecture` |
| Required component/system scenarios | `measix-architecture` |
| Stage reading lists | `measix-architecture/docs/measix-stage-document-index.md` |
| Exact executable HTTP schema | `api/*.openapi.yaml` |
| Canonical fixtures | `api/fixtures/` |
| Source/package/UI structure | source tree + local implementation docs |
| Concrete frontend/Go dependencies | package manifests/lockfiles |
| Ent schema / Atlas migrations | repository source |
| Build/test/CI/operations | repository tooling + local docs |
| Freeze/RC evidence | repository test/release artifacts |

Do not create local copies of architecture contracts, stage reading lists, protocol documents or identifier tables.

## Local docs

- `ARCHITECTURE.md` — implementation dependency boundaries.
- `docs/s0-execution-progress.md` — current implementation state/gaps only.
- `docs/admin-console-implementation.md` — concrete Admin implementation decisions.
- `docs/api-contracts.md` — executable contract/codegen/freeze workflow.
- `docs/development.md` — engineering workflow.
- `docs/testing.md` / `docs/tdd.md` — executable tests and TDD.
- `docs/database-migrations.md` — persistence migration workflow.
- `docs/operations.md` — runtime operations.
- `docs/release.md` — freeze/RC evidence composition.

## Synchronization

### Architecture semantic change

```text
architecture authority
→ OpenAPI/fixtures/tests
→ implementation
→ downstream consumers when affected
```

### Pure implementation change

Code layout, component decomposition, dependency choice, DB index, build tooling and UI internals stay in this repository unless they change an architectural boundary.

### S0.1 Client Contract Freeze

Before C7, Client OpenAPI/Snapshot v1 is pre-freeze. C7 pins the architecture commit, platform-core commit, Client OpenAPI/fixture hashes, Snapshot schema version, Admin build identity and required qualification/scenario evidence. S0.2 consumes that pinned baseline.

## Documentation rule

A local document should contain only information needed to implement, run, test or operate this repository. If a paragraph re-explains an architecture requirement without adding a local implementation consequence, replace it with a reference.

Stage-specific reading order is maintained only in `topabomb/measix-architecture/docs/measix-stage-document-index.md`.
