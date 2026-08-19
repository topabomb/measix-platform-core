# Development Workflow

This document defines how engineers work on `measix-platform-core` locally or entirely through GitHub. It does not redefine platform behavior.

## 1. Toolchain baseline

S0 architecture fixes the major implementation stack:

- Go 1.26.x;
- OpenAPI 3.0.3;
- `oapi-codegen/v2` / `kin-openapi`;
- SQLite with `modernc.org/sqlite`;
- Ent + Atlas versioned migrations;
- Vue 3 + TypeScript + Quasar;
- Pinia + Vue Router;
- `openapi-typescript`;
- pnpm.

Exact patch/tool versions become authoritative in repository-controlled toolchain files and lockfiles when I0 initializes them. Do not duplicate floating version tables across multiple Markdown files.

## 2. Development modes

### Local-first

A local checkout is the fastest Red/Green loop:

```text
branch
→ smallest failing test
→ run narrow test locally
→ implement
→ rerun narrow test
→ affected component suite
→ push / PR
→ GitHub CI independently verifies
```

Local configuration uses synthetic/test credentials only. Development must not require production configuration or public Provider access for normal T0–T3 work.

### GitHub-only

When development is performed through GitHub/API/coding-agent access without a local executor:

```text
branch
→ Draft PR
→ commit Red test
→ GitHub Actions executes and fails as expected
→ inspect failing check/log
→ commit implementation
→ GitHub Actions executes latest SHA and passes
→ inspect checks/artifacts
→ refactor / rerun
```

GitHub Actions is the executor in this mode. Static code review alone is not test execution.

See `docs/tdd.md` for the evidence contract.

## 3. I0 target repository structure

The implementation is organized around executable ownership, not around duplicating architecture documents:

```text
api/          executable wire contracts + canonical fixtures
backend/      Go binaries, packages, Ent, migrations
console/      Admin Console source/build
test/         qualification + S0 system harness
.github/      CI/PR automation
```

Subdirectories are created when their implementation lands. The source tree, not an old documentation snapshot, is authoritative for concrete package/file locations.

## 4. Bootstrap expectations

I0 must establish reproducible tool setup for both local CI-equivalent execution and GitHub Actions. Before I1 work begins, the repository must be able to:

- build `control-hub` and `runtime-relay` health skeletons;
- validate all four OpenAPI documents;
- reproduce generated Go/TS/Android wire artifacts or verify their exported generation inputs;
- replay SQLite migrations from an empty database;
- build the Quasar production shell;
- execute deterministic T0/T1/T2 CI.

The exact commands become part of repository tooling when those artifacts land. Documentation must be updated in the same PR that introduces or changes a command.

## 5. Branch/PR development

Use a short-lived branch for implementation work. Open a Draft PR early for multi-commit TDD and cross-component work.

The PR is the coordination object for:

- architecture linkage;
- Red/Green evidence;
- generated-code drift;
- migration review;
- required CI checks;
- review discussion;
- eventual release/test manifest references.

Direct pushes to `main` should stop once branch protection/required checks are enabled.

## 6. Code-generation workflow

For any OpenAPI/schema-derived artifact:

```text
change authoritative source
→ validate source
→ regenerate deterministically
→ inspect diff
→ run fixture/contract tests
→ run generated-drift check
→ commit source + expected generated artifacts together where repository policy requires them committed
```

Never make the generated output the first or only source of a protocol change.

## 7. Database workflow

Schema work follows:

```text
failing domain/repository/migration test
→ Ent schema change
→ Atlas migrate diff
→ review SQL
→ empty replay + upgrade test
→ implementation Green
```

See `docs/database-migrations.md`.

## 8. Frontend workflow

Admin Console development separates:

- generated Admin API types;
- API/problem/session infrastructure;
- Pinia workflow state;
- feature components;
- pages/layout.

Frontend tests may stub Hub for component-level T1/T2, but system/RC browser lanes use a real Hub as required by the architecture Testing Specs.

## 9. Runtime Relay workflow

Relay data-path changes must be tested against real HTTP/TCP boundaries for streaming, cancellation, header handling and forwarding behavior. In-memory mocks do not replace required Relay T2/T3 scenarios.

## 10. Keeping docs accurate

When implementation changes how developers actually build/run/test/operate the repository, update the owning implementation document in the same PR. When behavior meaning changes, update architecture first instead.
