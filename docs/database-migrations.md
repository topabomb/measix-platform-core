# Database initialization

MEASIX 尚未发布，当前结构是唯一支持的数据库版本。旧开发数据库或配置直接删除、重新初始化；不提供增量迁移、旧策略收养、版本回填或兼容恢复。目录名沿用 migrations，不代表存在需要支持的历史版本。

## 当前结构与职责

- Ent schema 定义业务结构，`backend/migrations/202609120001_current.sql` 是唯一完整初始化 SQL。
- `migrations.CurrentSQL()` 要求恰好一份 SQL；测试使用同一来源。
- Hub 启动不运行 ORM AutoMigrate，不静默修改已有 schema。Relay 本地 spool 独立于 `hub.db`。
- 开发 helper `go run ./cmd/devmigrate --db ../.data/hub.db` 仅用于独立开发数据库。它读取当前 SQL、校验本地初始化记录，重复执行当前版本幂等，SQL 与初始化记录在同一事务中提交。非当前结构需清理相应旧数据库后重建，不修复或收养历史。它不要求目录摘要文件；SQL 与数据库之间的漂移由 `devmigrate_revisions` 中记录的文件名与 sha256 检出。
- 报 `non-current schema/checksum`（或 `non-current database`、`non-current initialization record`）表示该库由当前 SQL 的**另一版本**初始化，属过期开发状态。`npm run setup` 会对 `.data/hub.db` 自动删除并重建；真机预设用 `npm run device:real:reset` 对其独立数据目录做同样处理。两者都只删除各自的开发库，不改动密钥。仅"无法识别"的库不会被自动删除，需人工确认。
- `maintenance.Check` 校验 Ent 所需表/列及 SQLite integrity/foreign keys，不比较全部索引或列类型。System/backup 报告 binary expected revision。

## 修改与验证

修改 Ent schema 后，同步更新完整初始化 SQL 与生成代码。检查唯一约束、外键、默认值、NULL、索引和事务语义，不增加旧数据回填脚本。

验证方式是执行真实 SQL 的 Go 测试（空库应用、当前业务读写、重复初始化和失败事务回滚），而不是维护一份独立的摘要文件。当前版本备份/恢复与完整性检查仍需保留，旧版本升级测试删除。

SQLite 连接配置由 `common/sqliteutil` 统一维护。

## 旧开发文件处理

先停止占用相应数据库的本地进程，确认路径属于本项目且文件确为旧版本，再删除数据库及其 `-wal`/`-shm` 文件，按当前初始化流程创建。旧配置直接移除并使用当前配置；不触碰 Android 或其他项目数据。不因某个测试失败就清空正常的当前数据库或密钥。

当前版本日常备份、恢复与密钥操作见 [operations.md](operations.md)。首次发布前另行确定发布后的持久化政策。

ManagedRelease 不再持久化重复的 snapshot_schema_version 列。当前 Snapshot JSON 的 schemaVersion 是协议版本来源，读取和编译统一校验当前版本；数据库不维护历史版本索引或回填流程。
