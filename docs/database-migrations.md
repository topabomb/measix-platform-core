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
