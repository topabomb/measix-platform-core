# Repository Guidelines for Coding Agents

This file is the execution contract for AI/coding agents working in `measix-platform-core`.

## 1. Read before implementation

Before changing behavior, read:

1. `docs/documentation-governance.md`;
2. `ARCHITECTURE.md`;
3. relevant documents in `topabomb/measix-architecture`.

For current S0 work, the minimum architecture reading order is:

1. `measix-s0-foundation-contract-spec.md`;
2. `measix-s0-capability-delivery-contract-spec.md` for S0.1 server-side capability-delivery work;
3. `measix-s0-capability-delivery-implementation-decision.md` for the S0.1 execution order;
4. `measix-s0-control-protocol.md` when executable wire/state/error semantics are affected;
5. the relevant Component Architecture / Implementation Spec / Testing Spec;
6. `measix-s0-capability-delivery-system-testing-spec.md` for the S0.1 pre-Android system gate;
7. `measix-s0-android-integration-contract-spec.md` when work affects the frozen Client Snapshot/OpenAPI handoff to S0.2;
8. `measix-s0-system-testing-spec.md` for final cross-repository S0 RC/Exit work.

`S0.1` and `S0.2` are delivery sub-stages inside S0. They do not rename S1/S2/S3 and they do not create a second local architecture authority in this repository.

Do not infer a new platform semantic from existing code when architecture already owns the meaning.

## 2. Change classification is mandatory

Before editing, decide whether the task changes architecture semantics, executable contracts, implementation only, or fixes a regression.

If the change alters terminology, stable IDs, state/wire/error semantics, security invariants, S0.1/S0.2 delivery gates or required S0 scenarios, update/resolve `measix-architecture` first. Do not silently encode a new semantic in Go, TypeScript, OpenAPI or tests.

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
- S0.1 Provider compatibility must remain outside Relay body semantics; Relay stays provider-agnostic.

## 7. Generated code and OpenAPI

Never hand-edit generated outputs. Change OpenAPI/generator source, regenerate, then verify drift.

A semantic wire change requires architecture approval first. A schema completion that preserves existing meaning may land here, but ambiguity must be escalated rather than guessed.

The Client Control OpenAPI is **pre-freeze until the S0.1 Client Contract Freeze Gate succeeds**. Before S0.2 Android implementation starts, the S0.1 freeze manifest must pin the architecture commit, platform-core commit, Client OpenAPI hash, canonical fixture hash and Snapshot schema version. After that freeze, incompatible changes require the architecture-defined compatibility/versioning path rather than silently mutating Snapshot v1.

## 8. Persistence and migrations

- no ORM AutoMigrate in production;
- published Atlas migrations are immutable;
- schema changes require migration generation/review and replay/upgrade tests;
- Relay never reads or writes `hub.db`.

## 9. Testing expectations

Architecture scenario IDs are requirements, not test implementation names. Reference `HUB-*`, `RLY-*`, `ADM-*`, `CAP-*` and `SYS-*` where the test proves a critical scenario; ordinary unit tests do not need artificial IDs.

S0.1 `CAP-*` scenarios prove the pre-Android server-side product closure using real Admin/Hub/Relay plus deterministic Test Client/Test Adapter and required real Adapter qualification. Final `SYS-*` S0 Exit still requires the pinned Android repository and Android emulator/device evidence defined by architecture.

Tests must be deterministic, isolated, bounded by deadlines, use synthetic credentials/content, and avoid public-network dependencies in normal PR CI.

## 10. Completion report

For implementation work, report:

- files/areas changed;
- architecture reference or `Architecture impact: none`;
- Red evidence;
- Green evidence;
- exact test layers/checks executed;
- current delivery sub-stage (`S0.1`, `S0.2` or final S0 RC) when relevant;
- remaining verification gaps;
- migrations/generated artifacts/operational impact when applicable.

Never state that S0.1, S0.2 or S0 Exit is complete unless the corresponding architecture gate has actually executed and its required evidence exists. Never state that a test passed unless it actually executed locally or in CI and the result was observed.
