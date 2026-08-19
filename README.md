# MEASIX Platform Core

`measix-platform-core` is the implementation repository for the MEASIX S0 server-side platform. It contains the executable contracts and source for **Control Hub**, **Runtime Relay**, and **Admin Console**, plus their shared test infrastructure.

## Architecture authority

Product semantics, platform terminology, S0 scope, cross-component behavior, component architecture, and required S0 test scenarios are **not redefined in this repository**.

The authoritative architecture repository is:

- `topabomb/measix-architecture`
- S0: `docs/10-runtime-foundation/s0/`

Start with `ARCHITECTURE.md` and `docs/documentation-governance.md` before making implementation changes.

## Repository responsibilities

This repository owns implementation facts and executable artifacts, including:

- OpenAPI contracts and canonical wire fixtures;
- generated Go / TypeScript artifacts;
- Control Hub and Runtime Relay source;
- Admin Console source and production build;
- Ent schema and Atlas versioned migrations;
- component integration tests;
- Upstream Adapter qualification harness;
- S0 cross-component/system test harness and reports;
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
| `docs/development.md` | local and GitHub-only development workflow |
| `docs/api-contracts.md` | OpenAPI, fixtures and code-generation ownership |
| `docs/testing.md` | executable testing conventions, CI layers and evidence |
| `docs/tdd.md` | mandatory TDD workflow, including GitHub-only Red/Green verification |
| `docs/database-migrations.md` | Ent / Atlas / SQLite migration workflow |
| `docs/operations.md` | runtime configuration, health, backup, restore and upgrade procedures |
| `docs/release.md` | release-candidate composition and reproducible S0 verification |

## Development principle

For behavior changes, development is test-driven:

```text
architecture requirement
  → failing executable test (Red)
  → minimum implementation (Green)
  → refactor while Green
  → required CI / system evidence
```

A change that alters platform semantics must first update the authoritative document in `measix-architecture`; a pure implementation change remains in this repository.
