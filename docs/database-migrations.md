# Database migrations

S0.2 Preview uses one ordered, append-only migration history for the Control Hub database. Production and development use the same embedded migration owner in `backend/migrations`.

## Contract

- Files are named `000001_name.sql`, `000002_name.sql`, and so on, with contiguous versions starting at 1.
- An accepted migration is immutable. Change the schema by adding the next file; never edit or reorder an applied file.
- `schema_migrations` records version, filename, SHA-256 and apply time. Startup and `check` reject missing history, gaps, changed checksums and databases newer than the binary.
- Every file and its history row commit in one transaction. Earlier completed files remain committed if a later file fails.
- Hub startup never mutates schema. Upgrade automation must back up first, then run `control-hub migrate`, then `control-hub check`, before starting the service.
- An unrecognized non-empty database is never adopted or deleted automatically.

The one pre-Preview development layout recorded in `devmigrate_revisions` may be adopted only when its exact filename and historical checksum match the known version-1 schema. Adoption replaces only the history table; it does not rewrite business data. Atlas-managed databases remain rejected.

## Commands

Production and packaged upgrades:

```text
control-hub migrate --db <hub.db>
control-hub check --db <hub.db>
```

Local development may continue to use the compatibility wrapper:

```text
go run ./cmd/devmigrate --db ../.data/hub.db
```

Both commands execute `migrations.Apply`; the wrapper does not have separate schema behavior. Repeating either command at the current version is a no-op.

## Schema changes

Update the Ent schema and generated code, add the next SQL file, and test at least:

1. empty database to current;
2. previous supported version to current with representative data preserved;
3. repeat application;
4. per-file rollback on invalid SQL;
5. checksum conflict, version gap and future-version rejection;
6. backup, isolated restore, `check`, and critical business reads.

SQLite connections remain owned by `internal/common/sqliteutil`. Relay spool schema is a separate owner and must be backed up or replay-qualified independently.

## 发布历史迁移

000004 保存 generation 高水位、独立保留规则与有界清理审计，并为保留版本增加差异摘要缓存。迁移从所有 Release、Activation target 和当前 generation 初始化高水位；从原 Snapshot 为全部 admission（含待核对请求）补录资源名称。后续 admission 在同一事务记录名称，不依赖可清理的发布记录。迁移不改现存发布内容、Snapshot/hash、用量或结算字节。

升级前须保留原包与完整备份；旧 binary 不接受 schema 4，回退需恢复迁移前备份，不能只切换 binary。清理移除逻辑发布数据，SQLite 空闲页供后续写入复用，不在在线服务中执行 VACUUM。升级与名称/编号保全由 `migrations/release_history_upgrade_test.go` 验证。


## 远程工作区迁移

000002 新增工作区控制数据；000003 为预算/用量增加互斥目标分支，保留已有归属和外键引用。带历史结算数据的升级及 foreign_key_check 由 `migrations/workspace_upgrade_test.go` 覆盖。不得编辑已部署迁移记录或以清库替代升级。

详见[远程工作区实现参考](remote-workspace-implementation.md)；历史联调只在[证据索引](s0-execution-progress.md)定位。
