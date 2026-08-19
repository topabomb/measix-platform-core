# Repository Guidelines for Coding Agents

This file is the execution contract for AI/coding agents working in `measix-platform-core`.

## 1. Read before implementation

Before changing behavior, read:

1. `docs/documentation-governance.md`;
2. `ARCHITECTURE.md`;
3. relevant documents in `topabomb/measix-architecture`:
   - S0 Foundation Contract;
   - Implementation Decision;
   - Control Protocol when cross-component;
   - relevant Component Architecture;
   - relevant Component Implementation Spec;
   - relevant Component Testing Spec;
   - S0 System Testing Spec for cross-component/system work.

Do not infer a new platform semantic from existing code when architecture already owns the meaning.

## 2. Change classification is mandatory

Before editing, decide whether the task changes architecture semantics, executable contracts, implementation only, or fixes a regression.

If the change alters terminology, stable IDs, state/wire/error semantics, security invariants or required S0 scenarios, update/resolve `measix-architecture` first. Do not silently encode a new semantic in Go, TypeScript, OpenAPI or tests.

## 3. TDD by default

For new behavior and bug fixes:

```text
Red → Green → Refactor
```

- add the smallest meaningful failing test first;
- confirm it fails for the intended reason;
- implement only enough to pass;
- refactor while preserving Green;
- run the complete affected gate before declaring completion.

Do not delete, skip, relax or retry-away the failing requirement.

Docs-only changes and pure generated-output regeneration do not require an artificial Red test, but generated drift/validation still must pass.

## 4. GitHub-only execution mode

When no local shell/runtime is available, do not pretend tests were run locally.

Use a branch + Draft PR and GitHub Actions as the test executor:

1. commit/push the Red test;
2. inspect the PR workflow run/check and verify the expected failure;
3. commit the minimum implementation;
4. inspect the latest commit's workflow run/check and verify Green;
5. inspect logs/artifacts for the affected test lane;
6. refactor and re-run;
7. only claim tests passed when GitHub reports the relevant checks successful on the latest commit SHA.

If CI cannot execute the required test, report that as a verification gap; do not substitute static inspection for an executed test.

## 5. Local execution mode

When a local checkout/runtime is available:

- run the narrow failing test continuously during Red/Green;
- run the affected package/component suite before push;
- let GitHub CI independently repeat the required gate;
- never treat a local pass as a reason to ignore a CI failure.

Exact commands are documented only after their corresponding tooling exists; use `docs/development.md` and `docs/testing.md` rather than inventing commands.

## 6. Dependency boundaries

- Runtime Relay must not import Hub domain/service/Ent packages.
- Admin Console must not call Relay internal APIs.
- generated wire types come from OpenAPI; do not hand-maintain duplicate DTOs.
- canonical cross-component fixtures belong under `api/fixtures/`.
- production code must not depend on test harness packages.

## 7. Generated code and OpenAPI

Never hand-edit generated outputs. Change OpenAPI/generator source, regenerate, then verify drift.

A semantic wire change requires architecture approval first. A schema completion that preserves existing meaning may land here, but ambiguity must be escalated rather than guessed.

## 8. Persistence and migrations

- no ORM AutoMigrate in production;
- published Atlas migrations are immutable;
- schema changes require migration generation/review and replay/upgrade tests;
- Relay never reads or writes `hub.db`.

## 9. Testing expectations

Architecture scenario IDs are requirements, not test implementation names. Reference `HUB-*`, `RLY-*`, `ADM-*` and `SYS-*` where the test proves a critical scenario; ordinary unit tests do not need artificial IDs.

Tests must be deterministic, isolated, bounded by deadlines, use synthetic credentials/content, and avoid public-network dependencies in normal PR CI.

## 10. Completion report

For implementation work, report:

- files/areas changed;
- architecture reference or `Architecture impact: none`;
- Red evidence;
- Green evidence;
- exact test layers/checks executed;
- remaining verification gaps;
- migrations/generated artifacts/operational impact when applicable.

Never state that a test passed unless it actually executed locally or in CI and the result was observed.
