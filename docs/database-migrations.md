# Database Migrations

This document is the implementation procedure for SQLite schema changes in `measix-platform-core`. Architecture owns persistence invariants; this repository owns the actual Ent schema, Atlas migrations and verification workflow.

## 1. Baseline

Control Hub uses:

```text
SQLite
+ Ent schema
+ Atlas versioned migrations
```

Production startup must not use ORM AutoMigrate. Runtime Relay owns its separate local persistence (for example usage spool) and does not share `hub.db`.

## 2. Source ownership

The repository is authoritative for:

- `backend/ent/` schema/code generation inputs;
- `backend/migrations/` versioned SQL;
- Atlas configuration and `atlas.sum`;
- migration bootstrap/apply tooling;
- empty-replay and upgrade tests.

Once a migration is published/merged as part of a released history, do not edit it to change the past. Add a new migration.

## 3. TDD migration workflow

For behavior that needs schema change:

```text
1. write repository/domain/migration test that demonstrates missing behavior
2. observe Red
3. change Ent schema
4. generate Atlas migration diff
5. review generated SQL manually
6. run migration validation
7. replay from empty DB
8. test upgrade from relevant previous schema/fixture
9. implement repository/domain behavior
10. observe Green
```

The exact order of schema generation and production-code implementation may vary inside the branch, but the final PR must contain an observed failing test before the behavior is considered proven.

## 4. Migration review

Review every generated SQL migration for:

- destructive table/column operations;
- unexpected table rebuilds;
- constraint/index semantics;
- default/backfill behavior;
- NULL/non-NULL transitions;
- stable ID uniqueness/foreign references where applicable;
- transaction/locking implications;
- compatibility with existing persisted rows;
- accidental dependence on SQLite behavior not exercised by tests.

Generated SQL is not accepted merely because Atlas produced it.

## 5. Required tests

A schema-changing PR must run as applicable:

### Empty replay

Apply all committed versioned migrations to a fresh SQLite file and start the owning component against it.

### Upgrade

Create/load a database at the relevant previous version, apply new migrations, and verify persisted domain facts remain correct.

### Failure/restart

For migrations with meaningful risk, test interrupted/failed application and ensure the operational procedure gives a diagnosable, recoverable result rather than silently starting on a partially valid schema.

### Schema checksum

Validate Atlas migration integrity/`atlas.sum` so old migrations cannot drift unnoticed.

## 6. Test data

Migration fixtures use synthetic data only. Never commit production database extracts or credentials.

Fixtures should be minimal and purpose-built: enough to prove preservation of IDs, references, releases/generations, usage or other affected facts without becoming a second canonical business fixture set.

## 7. Deployment order

The operational flow is:

```text
backup/checkpoint as required
→ apply versioned migration
→ verify migration result
→ start/upgrade component
→ readiness/health verification
```

The application must not silently mutate schema during normal startup.

## 8. Rollback philosophy

Do not assume every migration is safely reversible. Before a risky migration reaches RC, define whether recovery is:

- forward-fix with a subsequent migration;
- restore from verified backup;
- or an explicitly tested reverse operation.

`docs/operations.md` owns the concrete backup/restore execution once tooling is implemented.

## 9. Documentation boundary

Do not copy full Ent schema/SQL into architecture Markdown or this document. The checked-in source files are the executable truth. This document only defines the process and invariants for changing them.
