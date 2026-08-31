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

当前 SQL 历史包含初始 schema、Enterprise Update，以及 2026-08-31 的四份增量：Session rotation/device name、Feed timezone/revision、Semantic completeness EXACT、未命中路由 Usage 的可空归属。最新 expected revision 为 `202608310004`；完整清单由 `backend/migrations/source.go` 嵌入的 versioned SQL 派生，不再手工复制到 testutil。既有迁移语义与 checksum 保持不变。

`devmigrate` 为隔离开发环境使用 checksum + 有序 ledger；单份 SQL 和 ledger 写入同一事务，错误必须回滚，不忽略 already-exists。它拒绝 Atlas-managed、非连续/变更 checksum、无 checksum 的旧 ledger 或未跟踪的非空数据库。遇到旧开发库先备份并验证迁移归属；不能删库、自动收养历史或当作 Atlas production gate。新建独立开发库可以直接使用，旧库迁移需显式审查。

`maintenance.Check` 从 Ent generated schema 检查当前所有表/列及 SQLite integrity/foreign keys；System/backup 的 revision 是 binary expected revision，不冒充查询到的 Atlas applied revision。该检查不比较索引、列类型或完整 migration history；生产仍须 Atlas apply/status 和受控升级验收。

SQLite 打开策略集中在 `common/sqliteutil`：URL-escaped file path、每连接 foreign_keys/busy_timeout/WAL/FULL 配置、单连接池及 immediate transaction。换连接后也必须保持这些约束，业务包不自行维护另一套 PRAGMA 初始化。

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

Apply all committed versioned migrations to a fresh SQLite file and start the owning component against it. Test fixtures execute the embedded real SQL; upgrades use SQLAfter(previous revision), not a no-op fixture or ignored duplicate-table errors.

### Upgrade

Create/load a database at the relevant previous version, apply new migrations, and verify persisted domain facts remain correct.

### Failure/restart

For migrations with meaningful risk, test interrupted/failed application and ensure the operational procedure gives a diagnosable, recoverable result rather than silently starting on a partially valid schema.

### Schema checksum

Validate Atlas migration integrity/`atlas.sum` so old migrations cannot drift unnoticed.

CI-compatible POSIX 环境中，`make migration-replay` 对临时空库执行 Atlas apply/status；`make migrations` 再重算并比较 checksum。无 Atlas CLI 时可用 `go run ./cmd/migration-checksum` 重算 pinned Atlas library 的 checksum，并跑真实 SQL empty/upgrade tests，但不能称为 Atlas CLI gate。空库回放不等于真实旧版数据升级，后者必须另跑对应 fixture。不要在唯一生产库上试跑开发 helper 或未经审查的 migration diff。

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
