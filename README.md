# MEASIX Platform Core

S0 server-side implementation repository for **Control Hub**, **Runtime Relay**, **Enterprise Tool Gateway** and **Admin Console**.

Current source contains the Hub and Relay Go binaries plus the Admin SPA. The S0.3 Gateway binary, Gateway Control OpenAPI, production service units and unified production log baseline are planned but not implemented; see `docs/s0-execution-progress.md` for the exact current status.

## Start here

- Architecture stage reading list: `topabomb/measix-architecture/docs/measix-stage-document-index.md`
- Local implementation boundaries: `ARCHITECTURE.md`
- Current implementation/stage status: `docs/s0-execution-progress.md`
- Admin Console concrete implementation: `docs/admin-console-implementation.md`

## Repository ownership

This repository owns executable OpenAPI/fixtures, generated artifacts, Go services, Admin Console code, SQLite/Ent/Atlas migrations, tests, CI and operations.

Product semantics, stage scope, stable IDs, cross-component behavior and required stage scenarios are owned by `topabomb/measix-architecture`.

## Engineering docs

- `docs/development.md` — local/build/codegen workflow
- `docs/api-contracts.md` — OpenAPI/fixtures/codegen/freeze
- `docs/testing.md` — executable test/CI organization and TDD
- `docs/database-migrations.md` — migration workflow
- `docs/operations.md` — runtime/backup/restore
- `docs/release.md` — S0.1 freeze and final S0 RC evidence
- [2026-08-31 architecture/core audit](docs/architecture-alignment-audit.md) — source-backed deviation snapshot and remediation plan; not a living status or Freeze proof
