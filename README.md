# MEASIX Platform Core

S0 server-side implementation repository for **Control Hub**, **Runtime Relay**, **Enterprise Tool Gateway** and **Admin Console**.

The S0.2 `0.2.0-preview.22` composition is fixed; current source contains the Hub and Relay Go binaries plus the Admin SPA. The S0.3 Gateway binary and Gateway Control OpenAPI remain future work. See `docs/s0-execution-progress.md` for the pinned composition and evidence boundary.

## Start here

- Architecture stage reading list: `topabomb/measix-architecture/docs/measix-stage-document-index.md`
- Local implementation boundaries: `ARCHITECTURE.md`
- S0.2 sealed Preview identity and implementation status: `docs/s0-execution-progress.md`
- Admin Console concrete implementation: `docs/admin-console-implementation.md`

## Repository ownership

This repository owns executable OpenAPI/fixtures, generated artifacts, Go services, Admin Console code, the SQLite/Ent current schema, tests, CI and operations.

Product semantics, stage scope, stable IDs, cross-component behavior and required stage scenarios are owned by `topabomb/measix-architecture`.

## Engineering docs

- `docs/development.md` — local/build/codegen workflow
- `docs/api-contracts.md` — OpenAPI/fixtures/codegen/freeze
- `docs/testing.md` — executable test/CI organization and TDD
- `docs/database-migrations.md` — current database initialization workflow
- `docs/operations.md` — runtime/backup/restore
- `docs/s02-preview-deployment.md` — S0.2 internal Preview binary deployment and acceptance
- `docs/release.md` — S0.2 Preview composition, stage evidence and final S0 RC
- `docs/usage-budget.md` — protocol metering and user-budget implementation
- `docs/pricing-cost.md` — pricing and cost calculation
- `docs/real-device-preset.md` — local LAN real-supplier preset for Android device validation
- `docs/remote-workspace-integration-plan.md` — planned integration using existing Agent Space APIs, managed MCP, WebDAV file management/previews and administrator-provided DAV connection details
