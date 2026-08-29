# Repository Guidelines for Coding Agents

This repository implements MEASIX Control Hub, Runtime Relay, Enterprise Tool Gateway and Admin Console. The Gateway is an S0.3 target; always distinguish current source from planned architecture.

## Before changing behavior

1. Read `ARCHITECTURE.md` (includes documentation governance).
2. Use `topabomb/measix-architecture/docs/measix-stage-document-index.md` to identify the architecture documents for the current stage/workstream.

   **Local-first principle**: the architecture repository is cloned locally at `../measix-architecture` (relative to this repository root, i.e. a sibling directory). Always read architecture documents from the local checkout first. If the local checkout is missing, clone `git@github.com:topabomb/measix-architecture.git` (or the HTTPS equivalent) to that sibling path before proceeding. Do not infer architecture semantics from memory or from this repository's implementation alone.

3. Read the relevant local implementation document (`docs/api-contracts.md`, `docs/admin-console-implementation.md`, `docs/testing.md`, etc.).

Architecture owns product/wire/state/security meaning. Do not infer a different semantic from existing code.

## TDD

Behavior changes and bug fixes use:

```text
Red → Green → Refactor
```

- add the smallest meaningful failing test first;
- verify the intended failure;
- implement the minimum change;
- keep generated artifacts/migrations synchronized;
- run the affected gate and observe the latest result before reporting Green.

Docs-only changes do not require artificial Red tests.

## Boundaries

- Runtime Relay must not import Hub domain/Ent packages or access `hub.db`.
- Enterprise Tool Gateway must not import Hub durable domain/Ent packages, access `hub.db`, accept public Android auth directly or become a fallback for Direct Managed MCP.
- Admin Console calls only the Control Hub Admin API; never Relay internal APIs.
- generated wire types come from OpenAPI; do not maintain duplicate DTOs.
- canonical cross-component fixtures live under `api/fixtures/`.
- Relay remains provider-agnostic; provider-specific body translation does not belong there.
- Secret plaintext must not enter persistent browser state or logs.

## Contract changes

A semantic wire/state/ID/security change requires architecture authority first. Then update, as applicable:

```text
OpenAPI → fixtures → generated artifacts → tests → implementation
```

Snapshot v1 is frozen only for the exact candidate pinned by the S0.1 manifest. Later v2/v3 additions and incompatible changes require the architecture-defined compatibility/versioning decision and their own executable evidence.

## Frontend dependencies

Normal Vue/browser package selection belongs to this repository. `console/package.json` and `pnpm-lock.yaml` are the concrete dependency authority. Do not require an architecture whitelist for ordinary helpers, visualization, date/time, accessibility or server-state packages.

New dependencies must have a real feature use, avoid duplicating Quasar or business/wire authority, and pass typecheck/test/build. See `docs/admin-console-implementation.md`.

## Completion claims

Do not claim S0.1, S0.2 or S0 Exit unless the corresponding architecture gate has executed and the required evidence exists. Historical Green runs are regression evidence only; they do not satisfy a newer head commit.

Implementation reports should state changed areas, architecture impact, Red/Green evidence, executed checks and remaining gaps.
