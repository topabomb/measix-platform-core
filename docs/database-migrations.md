# Database initialization

MEASIX 尚未发布，当前结构是唯一支持的数据库版本。旧开发数据库或配置直接删除、重新初始化；不提供增量迁移、旧策略收养、版本回填或兼容恢复。目录名沿用 migrations 以便 Atlas 使用，不代表存在需要支持的历史版本。

## 当前结构与职责

- Ent schema 定义业务结构，`backend/migrations/202609120001_current.sql` 是唯一完整初始化 SQL，`atlas.sum` 校验其内容。
- `migrations.CurrentSQL()` 要求恰好一份 SQL；测试使用同一来源。
- Hub 启动不运行 ORM AutoMigrate，不静默修改已有 schema。Relay 本地 spool 独立于 `hub.db`。
- 正式初始化使用 Atlas apply/status；开发 helper `go run ./cmd/devmigrate --db ../.data/hub.db` 仅用于独立开发数据库。它校验当前 SQL 和本地初始化记录，重复执行当前版本幂等，SQL 与初始化记录在同一事务中提交。非当前结构需清理相应旧数据库后重建，不修复或收养历史。
- `maintenance.Check` 校验 Ent 所需表/列及 SQLite integrity/foreign keys；它不是 Atlas ledger 检查，不比较全部索引或列类型。System/backup 报告 binary expected revision。

## 修改与验证

修改 Ent schema 后，同步更新完整初始化 SQL、生成代码和 checksum。检查唯一约束、外键、默认值、NULL、索引和事务语义，不增加旧数据回填脚本。

执行 `go run ./cmd/migration-checksum`；空库应用真实 SQL，验证当前业务读写、重复初始化和失败事务回滚。当前版本备份/恢复、完整性检查仍需保留，旧版本升级测试删除。

`node scripts/checks.mjs migration-replay` 使用独立临时数据库执行 Atlas apply/status，结束后清理。无 Atlas CLI 时 Go SQL 测试不能冒充 Atlas CLI gate。SQLite 连接配置由 `common/sqliteutil` 统一维护。

## 旧开发文件处理

先停止占用相应数据库的本地进程，确认路径属于本项目且文件确为旧版本，再删除数据库及其 `-wal`/`-shm` 文件，按当前初始化流程创建。旧配置直接移除并使用当前配置；不触碰 Android 或其他项目数据。不因某个测试失败就清空正常的当前数据库或密钥。

当前版本日常备份、恢复与密钥操作见 [operations.md](operations.md)。首次发布前另行确定发布后的持久化政策。

ManagedRelease 不再持久化重复的 snapshot_schema_version 列。当前 Snapshot JSON 的 schemaVersion 是协议版本来源，读取和编译统一校验当前版本；数据库不维护历史版本索引或回填流程。
