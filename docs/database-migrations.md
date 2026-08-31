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

当前迁移为 `202608190001_initial.sql` 与 `202608280001_enterprise_updates.sql`（完整清单始终以 `backend/migrations/` 为准）。`backend/cmd/devmigrate` 使用自己的 `devmigrate_revisions` 表，不是 Atlas history；遇到 `already exists` 的容错不能证明整份迁移完整，因此只作为隔离开发环境便利工具，不替代 release migration gate。

`maintenance.Check` 目前只检查初始 17 张表及 SQLite integrity/foreign keys，未校验 Enterprise Update 表、完整列/索引和 migration history。`CurrentSchemaRevision` 与 backup metadata 仍固定初始值，不能当作运行库最新 schema 的证明。相关修复见 [alignment audit](architecture-alignment-audit.md)。

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

CI-compatible POSIX 环境中，`make migration-replay` 对临时空库执行 Atlas apply/status；`make migrations` 再重算并比较 checksum。空库回放不等于真实旧版数据升级，后者必须另跑对应 fixture。不要在唯一生产库上试跑开发 helper 或未经审查的 migration diff。

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
