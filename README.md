# MEASIX Platform Core

`measix-platform-core` is the implementation repository for the MEASIX S0 server-side platform. It contains the executable contracts and source for **Control Hub**, **Runtime Relay**, and **Admin Console**, plus their shared test infrastructure.

## Architecture authority

Product semantics, platform terminology, S0 scope, cross-component behavior, component architecture, Admin product/UX requirements, delivery gates and required S0 test scenarios are **not redefined in this repository**.

The authoritative architecture repository is:

- `topabomb/measix-architecture`
- S0 documents: `docs/10-runtime-foundation/s0/`
- current Admin product/UX baseline: architecture commit `6de9bfb794e60e9bb6c62501263cc1518e4f5ee3`

Current S0 delivery order is governed by architecture as:

```text
S0 Core foundation
  → S0.1 Managed Capability Delivery
  → S0.2 Android Managed Runtime Integration
  → S0 Exit
```

`S0.1` and `S0.2` are delivery sub-stages inside S0; S1 remains Agent Space.

Start with `ARCHITECTURE.md`, `docs/documentation-governance.md`, `docs/s0-execution-progress.md` and `docs/admin-console-implementation.md` when working on the Admin surface.

## Current implementation status

The active implementation branch/PR contains most of the original S0 Core backend foundation, but **must not be described as S0-complete or Android-ready yet**.

The current implementation target is **S0.1 Managed Capability Delivery**: complete the Admin → Hub → Relay → Adapter → Usage/Cost product loop, align and freeze the Client Snapshot/OpenAPI contract, pass the S0.1 pre-Android system gate, and produce the freeze manifest required before S0.2 Android work starts.

For Admin Console specifically, the architecture now owns a formal product/UX contract for long-term information architecture, modern interaction, visualization and S0.1 golden-path requirements; this repository owns the concrete Vue/Quasar code structure and frontend dependency choices.

See `docs/s0-execution-progress.md` for the authoritative implementation status and remaining gaps in this repository.

## Repository responsibilities

This repository owns implementation facts and executable artifacts, including:

- OpenAPI contracts and canonical wire fixtures;
- generated Go / TypeScript artifacts;
- Control Hub and Runtime Relay source;
- Admin Console source, concrete frontend dependencies and production build;
- Ent schema and Atlas versioned migrations;
- component integration tests;
- Upstream Adapter qualification harness;
- S0.1 pre-Android capability-delivery system harness;
- final S0 cross-component/system test harness and reports;
- CI workflows, build, local-development and operational procedures.

## Target structure

```text
measix-platform-core/
├── api/
│   ├── admin/
│   ├── client/
│   ├── internal/
│   └── fixtures/
├── backend/
│   ├── cmd/control-hub/
│   ├── cmd/runtime-relay/
│   ├── pkg/platformid/
│   ├── internal/hub/
│   ├── internal/relay/
│   ├── ent/
│   └── migrations/
├── console/
├── test/
│   ├── qualification/
│   └── system/
├── docs/
└── .github/
```

The structure above is the S0 implementation target. Concrete directories are created only when their implementation lands; documentation must not pretend an unimplemented command or artifact already exists.

## Documentation

| Document | Purpose |
|---|---|
| `ARCHITECTURE.md` | implementation architecture boundaries and dependency rules |
| `AGENTS.md` | repository rules for AI/coding agents |
| `CONTRIBUTING.md` | contribution, PR and change-classification workflow |
| `docs/documentation-governance.md` | documentation authority and synchronization with `measix-architecture` |
| `docs/s0-execution-progress.md` | current S0/S0.1 implementation plan, evidence and remaining gaps |
| `docs/admin-console-implementation.md` | concrete Admin Console Vue/Quasar structure, UX implementation patterns and dependency strategy |
| `docs/development.md` | local and GitHub-only development workflow |
| `docs/api-contracts.md` | OpenAPI, fixtures, code generation and S0.1 client-contract freeze ownership |
| `docs/testing.md` | executable testing conventions, S0.1/S0.2 gates, CI layers and evidence |
| `docs/tdd.md` | mandatory TDD workflow, including GitHub-only Red/Green verification |
| `docs/database-migrations.md` | Ent / Atlas / SQLite migration workflow |
| `docs/operations.md` | runtime configuration, health, backup, restore and upgrade procedures |
| `docs/release.md` | S0.1 freeze-candidate and final S0 release-candidate composition/evidence |

## Development principle

For behavior changes, development is test-driven:

```text
architecture requirement
  → failing executable test (Red)
  → minimum implementation (Green)
  → refactor while Green
  → required CI / system evidence
```

A change that alters platform semantics must first update the authoritative document in `measix-architecture`; a pure implementation change, including normal frontend dependency choices, remains in this repository unless it changes the architecture/product/security boundary.
