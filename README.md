# MEASIX Platform Core

S0 server-side implementation repository for **Control Hub**, **Runtime Relay** and **Admin Console**.

## Start here

- Architecture stage reading list: `topabomb/measix-architecture/docs/measix-stage-document-index.md`
- Local implementation boundaries: `ARCHITECTURE.md`
- Current S0.1 status: `docs/s0-execution-progress.md`
- Admin Console concrete implementation: `docs/admin-console-implementation.md`

## Repository ownership

This repository owns executable OpenAPI/fixtures, generated artifacts, Go services, Admin Console code, SQLite/Ent/Atlas migrations, tests, CI and operations.

Product semantics, stage scope, stable IDs, cross-component behavior and required stage scenarios are owned by `topabomb/measix-architecture`.

## Engineering docs

- `docs/development.md` — local/build/codegen workflow
- `docs/api-contracts.md` — OpenAPI/fixtures/codegen/freeze
- `docs/testing.md` — executable test/CI organization
- `docs/tdd.md` — Red → Green → Refactor
- `docs/database-migrations.md` — migration workflow
- `docs/operations.md` — runtime/backup/restore
- `docs/release.md` — S0.1 freeze and final S0 RC evidence
- `docs/documentation-governance.md` — documentation ownership/synchronization
