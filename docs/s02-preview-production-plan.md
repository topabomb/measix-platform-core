# S0.2 Preview 生产化实施方案与部署手册

> 状态：本批实施权威方案；完成实现、验证与独立审查后转为已完成记录。  
> 范围：当前 S0.2 / Snapshot v4 内部预览版；Control Hub、Runtime Relay、Admin Console、标准 Enterprise Portal。  
> 部署基线：单台 NVIDIA DGX Spark（Linux ARM64 / aarch64）、Caddy 反向代理和 TLS、root PM2 daemon 管理两个业务进程、预编译 Go 二进制和预构建静态资源。
> 根目录原则：除 Caddy/PM2 自身的系统安装和 daemon 元数据外，MEASIX 拥有的发布、配置、密钥、数据、日志、备份及运行文件全部位于一个 `MEASIX_ROOT` 下。

## 1. 目标与完成定义

本批交付一个可供内部真实使用的 S0.2 Preview。它必须同时满足：

1. 当前 Android 可见协议有固定基线和简单、明确的修改规则；
2. Hub、Relay 的关键运行事件、近期请求遥测和既有业务状态可在 Admin 中观察；
3. 配置、Hub 数据库和 Relay spool 在升级、重启和恢复时不会被发布目录覆盖；
4. 数据库从本 Preview 开始支持有序、前向、可校验的升级；
5. 发布物是 Linux 二进制和静态资源包，服务器不需要源码、Go、pnpm 或 `node_modules`；
6. Caddy 提供唯一 HTTPS 入口，`sudo pm2` 管理 Hub/Relay 生命周期；
7. 在独立 Linux 环境完成安装、升级、备份、恢复、回退和 Android 主流程验收；
8. 全部实现完成后由独立子代理按本文逐项审查，修复其确认的问题并重跑受影响验证。

本批不实现 Gateway、Snapshot v5、HA、多节点数据库、Prometheus、集中日志平台、多协议并行兼容、自动密钥轮换或全设备矩阵。

## 2. 当前基线与所有权

### 2.1 协议基线

当前唯一支持的预览协议为：

| 合同 | 当前版本/来源 |
| --- | --- |
| Client HTTP | `/api/client/v1`，`api/client/client-control.openapi.yaml` |
| Runtime HTTP/WebSocket | `/runtime/v1`，Client + Relay control 定义 |
| Managed Snapshot | `schemaVersion=4` |
| Portal Native Bridge | `bridgeVersion=3` |
| Enrollment material | `formatVersion=1` |
| Admin HTTP | `api/admin/admin.openapi.yaml` |
| Hub ↔ Relay control | `api/internal/relay-control.openapi.yaml` |
| Relay → Hub usage | `api/internal/usage-ingest.openapi.yaml` |
| Android/Portal inputs | `api/generated/android/`、`api/portal/` 与 canonical fixtures |

Architecture 仓库拥有跨组件语义；上述 OpenAPI 拥有精确 HTTP/wire 形状；fixture 是跨消费者的固定示例；生成代码不得手工修改。

### 2.2 当前运行与持久化

- 当前生产进程只有 `control-hub` 和 `runtime-relay`；
- Admin 和标准 Portal 是静态资源，由 Hub 提供；
- Hub 拥有 `hub.db`；Relay 只拥有 `relay-spool.db`；二者不得互读数据库；
- Hub 已提供 `check` 和 `backup`；备份使用 SQLite `VACUUM INTO`；
- 当前数据库只有一份开发期初始化 SQL，尚无已发布版本的前向迁移；
- Hub/Relay 已使用 `slog.JSONHandler`，但事件覆盖、共同字段、关联和脱敏验证不足；
- Admin System 已有版本、DB health/schema identity、generation/revision/hash、Activation、Relay ready、spool 与 Usage ingest 状态。

## 3. 单根目录部署布局

部署者必须在安装时显式选择一个独立绝对路径作为 `MEASIX_ROOT`。程序、部署脚本和 PM2 模板不提供 `/opt`、`/srv` 或用户目录下的默认值；缺失时直接失败。一次部署只允许一个根目录。所有命令先解析并校验该绝对路径，不接受 `~`、相对路径或空值。

```text
<MEASIX_ROOT>/
├── current -> releases/0.2.0-preview.N/
├── releases/
│   └── 0.2.0-preview.N/
│       ├── bin/
│       │   ├── control-hub
│       │   └── runtime-relay
│       ├── assets/
│       │   ├── admin/
│       │   └── portal/
│       ├── deploy/
│       │   ├── ecosystem.config.cjs
│       │   ├── Caddyfile.template
│       │   └── *-preview.sh
│       ├── release.json
│       └── SHA256SUMS
├── config/
│   ├── ecosystem.config.cjs
│   ├── Caddyfile
│   └── config-version
├── secrets/
│   ├── master.key
│   ├── jwt-ed25519.seed
│   ├── relay-service.token
│   └── initial-admin-password
├── data/
│   ├── hub/hub.db
│   └── relay/relay-spool.db
├── logs/
│   ├── hub.jsonl
│   ├── hub.stderr.log
│   ├── relay.jsonl
│   └── relay.stderr.log
├── backups/
├── staging/
└── run/
```

权限：

- `releases/`、`current`、`config/`：root 写，业务进程只读；
- `secrets/`：root 管理，业务用户 `measix` 组只读，目录 `0750 root:measix`、文件 `0640 root:measix`；
- `data/`、`logs/`、`backups/`、`staging/`、`run/`：`measix:measix`，目录 `0750`；
- root PM2 daemon 通过 `uid`/`gid` 以 `measix` 用户启动 Hub/Relay；
- Caddy 的系统 binary、service unit、证书存储和 root PM2 daemon 自身的 `$PM2_HOME` 是外部运行依赖，不保存 MEASIX 业务配置或数据；`/etc/caddy/Caddyfile` 只允许链接到 `$MEASIX_ROOT/config/Caddyfile`。

发布、升级、回退不得删除或替换 `config/`、`secrets/`、`data/`、`logs/`、`backups/`。

## 4. 协议稳定性与扩展规则

### 4.1 Preview Contract Baseline

构建时生成 `release.json`，至少记录：

```text
releaseVersion
coreCommit
architectureCommit
portalCommit
androidCommit
buildTime
os
arch
snapshotSchemaVersion = 4
portalBridgeVersion = 3
enrollmentFormatVersion = 1
clientOpenApiHash
adminOpenApiHash
relayControlOpenApiHash
usageIngestOpenApiHash
androidManifestHash
portalManifestHash
canonicalFixtureHash
databaseSchemaVersion
artifacts[] { path, sha256 }
```

`release.json` 只记录来源和 hash，不复制协议字段或枚举定义。`SHA256SUMS` 覆盖包中所有二进制、静态资源、部署模板和 release manifest。

### 4.2 修改规则

同一当前 `/v1` 内允许：

- 新 endpoint；
- response 新增可忽略的 optional field；
- request 新增 optional field，前提是缺失语义明确且旧请求保持原行为；
- 不改变消费者解释的校验补齐。

以下属于 breaking change，必须先修改 Architecture，并显式升级相应协议/schema version：

- 删除字段或改变字段含义；
- optional 改 required；
- 改变稳定 ID、状态、错误码或幂等语义；
- enum 新值，而已有消费者没有 unknown-value 策略；
- 改变 canonical bytes/hash；
- 将 internal 字段暴露给 Android/Portal；
- 修改 Snapshot v4 中已有结构的解释。

内部 Preview 不维护多版本并行。breaking upgrade 由服务器、Portal 和 Android 协调升级；发布记录明确所需 Android build。

### 4.3 实施工作

1. 修复 Core、Portal、Android 生成包和 source hash 漂移；
2. 增加轻量 release manifest 生成器；
3. 将 OpenAPI parse、fixture、generate/drift、Portal contract、Android exported input hash 纳入 Preview release gate；
4. 为 additive/breaking 规则补充合同测试；
5. 更新 `docs/api-contracts.md` 和发布说明，但不创建第二份 schema 权威。

## 5. 结构化日志

### 5.1 输出与存储

Hub/Relay 只向 stdout 输出一行一个 JSON 对象；panic/runtime stderr 保留原文。PM2 分别写入 `$MEASIX_ROOT/logs`，并通过 `pm2-logrotate` 控制大小和保留期。

共同必填字段：

```text
time
level
msg
service          hub | relay
buildVersion
event
```

按事件适用的 optional 字段：

```text
requestId
interactionId
activationId
deploymentId
managedGeneration
controlRevision
resourceId
upstreamId
durationMs
httpStatus
upstreamHttpStatus
outcome
errorCode
```

日志只记录 route template，不记录 query string。字段长度需要有界；未知 error 的输出必须经过统一安全格式化。

绝对禁止：

- access/refresh/service token、Cookie、CSRF、Enrollment code；
- master/JWT/Provider credentials；
- prompt、请求/响应 body、工具参数/结果、音频或图片内容；
- private endpoint、JWT claims、toolRef；
- username、display name 等直接身份信息。

### 5.2 事件范围

Hub 的内部 Preview 最小事件集：

- `service.started`、`service.stopping`、`service.start_failed`；
- `command.failed`、`database.backup_*`、`database.migration_*`；
- `runtime.reconcile_failed`；
- Admin/Client 请求结束事件及认证/授权拒绝结果。

Relay 的内部 Preview 最小事件集：

- `service.started`、`service.stopping`、`service.start_failed`；
- `control.apply_completed`、`control.apply_failed`；
- `http.request_completed`；
- `usage_spool_write_failed`、`spool.flush_failed`、`spool.flush_recovered`；
- `spool.backup_*`；
- `shutdown.flush_incomplete`。

每个正常请求最多一条完成日志。`/live`、`/ready` 和成功的 Relay status 轮询不写日志也不进入请求遥测；失败、超时、拒绝和关键状态变化仍记录。Activation、预算、spool backlog 等已有结构化状态不重复制造领域事件，先由 System Status/telemetry 和请求完成结果联合排查；后续只有出现实际诊断缺口才增加事件。

### 5.3 实现结构

新增一个 Hub/Relay 共用但不含业务语义的 `internal/common/observability`：

- 统一构造 `slog.Logger`；
- 固定 `service`、`buildVersion`；
- 规范 event、duration/outcome/errorCode；
- 安全 error 格式化和字段截断；
- HTTP status/duration middleware；
- 日志 forbidden-material 测试辅助。

Hub/Relay 业务 owner 在状态改变处记录领域 event；通用 middleware 不猜业务语义。

## 6. 近期遥测

近期遥测用于内部诊断，不是持久业务账本。每个进程在内存保存最近 60 个一分钟桶，重启后从空窗口开始，并显式返回 `startedAt`。

每个进程统计：

```text
requestCount
successCount
clientErrorCount
serverErrorCount
rejectedCount
timeoutCount
cancelledCount
durationP95Ms
inFlight
```

Relay 额外统计 `upstreamErrorCount`、`budgetDeniedCount`；Hub 额外统计 `activationFailureCount`、`reconcileFailureCount`。现有 spool pending/oldest age、Usage ingest lag、semantic unknown/orphan 继续由 System Status 提供。

实现约束：

- 只有固定 60 个桶；
- duration 使用固定 histogram bucket 计算 P95，不保留每个请求；
- 不按 user/request/resource/upstream 建立无界标签；
- `/live`、`/ready` 不进入业务请求统计；
- Relay telemetry 作为 `ControlStatus` 的 additive optional 对象返回给 Hub；
- Hub 后台 reconciler 更新并缓存最近一次 Relay status；Admin 读取缓存，基础状态每 15 秒可见刷新，但不制造高频 private 请求和自观测噪音。`DBHealth` 的完整 integrity/history/column 检查缓存 60 秒，避免 UI 轮询反复执行重型 PRAGMA；命令行 `control-hub check` 始终执行即时完整检查。

## 7. Admin 观察与分析

### 7.1 API

新增：

```text
GET /api/admin/v1/system/telemetry?window=15m|60m
GET /api/admin/v1/system/events?service=HUB|RELAY&level=&event=&correlation=&limit=
```

`system/telemetry` 返回 Hub/Relay 各自 summary 和最多 60 个时间桶。`system/events` 只读取固定日志文件：

```text
$MEASIX_ROOT/logs/hub.jsonl
$MEASIX_ROOT/logs/hub.stderr.log
$MEASIX_ROOT/logs/relay.jsonl
$MEASIX_ROOT/logs/relay.stderr.log
```

Hub 增加 `--diagnostics-log-dir` / `HUB_DIAGNOSTICS_LOG_DIR`。服务端不接受客户端文件路径，只允许固定文件名。

日志读取限制：

- 每次最多从每个文件尾部读取 2 MiB；
- 返回最多 200 条，默认 100 条；
- 单行最大 4 KiB，超长截断并标记；
- JSON stdout 转为结构化事件；stderr 转为 `process.stderr` 事件；
- 不返回宿主文件路径；
- API 再执行一次 forbidden-key 过滤；
- 仅认证 Admin 可调用。

### 7.2 UI

System 页面保持一个 owner，调整为四个页签：

1. 概览：public origin、Portal、Hub/Relay build、DB health；
2. 配置交付：generation、revision/hash、Activation 与收敛；
3. 请求与计量：15/60 分钟请求、错误率、P95、并发、spool 与 Usage；
4. 近期事件：Hub/Relay 日志。

渲染约束：

- telemetry 每 15 秒刷新，页面不可见时暂停；
- 时间序列固定最多 60 点，使用轻量 SVG/CSS，不新增图表依赖；
- event 默认 100、最大 200，使用 Quasar virtual scroll；
- event 页面每 15 秒增量刷新，可暂停和手动刷新；
- 过滤改变后替换列表，不累积无限 DOM；
- 默认只展示 time/service/level/event/message，展开后展示关联字段；
- 未观测值显示未知，不显示 0；进程重启后明确提示窗口重置。

## 8. 数据库迁移

### 8.1 模型

从本 Preview 起，Hub 数据库采用 append-only forward migration：

```text
schema_migrations(
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  checksum TEXT NOT NULL,
  applied_at DATETIME NOT NULL
)
```

迁移文件使用递增版本，例如：

```text
000001_initial.sql
000002_add_*.sql
```

规则：

- 已发布 migration 不得修改；
- 每个 migration 记录 SHA-256；
- migration 按 version 顺序执行；
- 每个 migration 单独事务提交；
- checksum 不符、version 缺口或数据库版本高于 binary 时 fail closed；
- 不实现 down migration；
- schema identity 由有序 migration 名称和 bytes 共同计算；
- Ent schema、migration SQL、check 测试同步维护。

### 8.2 命令

生产 `control-hub` 增加：

```text
control-hub migrate --db <path>
control-hub check --db <path>
control-hub backup --db <path> --output <new-file>
```

`migrate` 对空库应用全部 migration；对旧库只应用 pending migration。`run` 永不自动迁移，数据库落后/超前/校验失败时拒绝启动。

开发 `devmigrate` 改为调用同一 migration owner，避免开发与生产维护两套语义。

### 8.3 验证

- 空库初始化；
- v1 到后续测试 migration 的数据保持；
- migration 中途失败回滚；
- 重复 migrate 幂等；
- checksum 篡改拒绝；
- version 缺口/未来版本拒绝；
- startup 对非当前数据库 fail closed；
- backup metadata 与 migration/schema identity 一致。

## 9. 配置、备份与恢复

### 9.1 配置

不新增通用 YAML 配置层。站点配置由 `$MEASIX_ROOT/config/ecosystem.config.cjs` 统一保存，秘密只保存为文件路径。

`$MEASIX_ROOT/config/config-version` 初始为 `1`。新增 optional 配置不提升版本；删除、改义或新增 required 配置才提升版本并在升级手册中给出明确转换。

数据库中的 Deployment name/public origin 等仍由 Admin 和 Hub 持久化 owner 管理；PM2 中的 public origin 只作为空库首次启动 seed，不能覆盖已经审核的数据库值。

### 9.2 备份范围

日常 Hub 热备份：

```text
$MEASIX_ROOT/backups/<timestamp>/hub.db
$MEASIX_ROOT/backups/<timestamp>/hub.db.metadata.json
```

完整 recovery set 同时包括：

```text
config/
secrets/
data/relay/relay-spool.db（由 Relay-owned `VACUUM INTO` 在线生成一致性镜像，不直接复制 WAL 主文件）
backup-manifest.json
```

manifest 固定记录 release version/四仓 commit、config/schema version、release manifest/checksum identity，以及每个恢复文件的 hash/size/mode。备份目录权限为 `0700`。生产环境应另行复制一份到主机外安全位置；本仓库只负责生成和验证本地备份。

### 9.3 恢复

恢复流程固定为：

1. `sudo pm2 stop measix-hub measix-relay`；
2. 将当前数据移到 `$MEASIX_ROOT/staging/recovery-<timestamp>/original/`，不直接覆盖删除；
3. 将备份复制到 staging 新路径；
4. 使用备份记录的 binary 执行 `control-hub check`；
5. 核对 config version、schema version、key files 和 release identity；
6. 原子切换验证后的 Hub DB；需要完整时间点恢复时同时切换 spool；
7. 启动 Relay、Hub；
8. 验证 readiness、generation/revision、Release、用户、Usage 和一次真实 Runtime 调用；
9. 验收完成前保留 original。

如新版本已执行数据库 migration，旧 binary 不得直接打开新 DB。回退必须恢复升级前备份，再切换旧 release。

## 10. 二进制发布包

本 Preview 的正式发布目标只有 DGX Spark 使用的 Linux ARM64；amd64 构建只可用于开发便利，不计入发布验收。包名：

```text
measix-core-<version>-linux-arm64.tar.gz
```

构建要求：

- locked Go/Node/pnpm 版本；
- `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` 构建 DGX Spark 目标二进制；
- Go `-trimpath`；
- `-ldflags` 注入非 `dev` 的 buildVersion；
- Admin production SPA；
- Enterprise Portal production SPA；
- 不包含源码、`.git`、`node_modules`、测试产物、数据库或秘密；
- 生成 `release.json` 和 `SHA256SUMS`；
- 从解压后的包运行 smoke，证明包不依赖源码目录。

服务器只需要 Linux、Caddy、Node + PM2；无需 Go 或 pnpm。

## 11. PM2 配置

`$MEASIX_ROOT/config/ecosystem.config.cjs` 以 root 所有、业务只读保存。两个 app 均使用 `exec_mode: 'fork'`、`instances: 1`、`interpreter: 'none'`。

固定端口：

| 服务 | 地址 |
| --- | --- |
| Caddy public | `:80` / `:443` |
| Hub public | `127.0.0.1:9004` |
| Hub internal | `127.0.0.1:9001` |
| Relay public | `127.0.0.1:9002` |
| Relay internal | `127.0.0.1:9003` |

PM2 关键参数：

```text
uid/gid: measix
autorestart: true
restart_delay: 3000
max_restarts: 10
min_uptime: 10000
kill_timeout: 40000
listen_timeout: 15000
```

日志：

```text
out_file:   $MEASIX_ROOT/logs/<service>.jsonl
error_file: $MEASIX_ROOT/logs/<service>.stderr.log
time: false
```

安装 `pm2-logrotate`：单文件 20 MiB、保留 7 份、压缩、每日检查。应用 JSON 已含时间，不让 PM2 添加文本前缀。

PM2 app 列表先 Relay 后 Hub；启动顺序不是 correctness 前提，Hub reconciler 仍负责恢复收敛。

## 12. Caddy 配置

`$MEASIX_ROOT/config/Caddyfile` 使用明确站点域名和 loopback upstream：

```text
https://core.example.com {
    @private path /internal /internal/*
    handle @private { respond 404 }

    @runtime path /runtime/v1 /runtime/v1/*
    handle @runtime {
        reverse_proxy 127.0.0.1:9002
    }

    handle {
        reverse_proxy 127.0.0.1:9004
    }
}
```

要求：

- `/internal` 永不暴露；
- 不配置 retry、body buffering 或 `flush_interval -1`；
- Caddy 自动处理 SSE/WebSocket 和 TLS；
- 保留 Caddy 默认仅 loopback 的 admin endpoint，供 systemd `reload` 使用，不对外开放；
- 只开放主机 80/443；四个业务端口仅 loopback；
- `/etc/caddy/Caddyfile` 链接到 `$MEASIX_ROOT/config/Caddyfile`；
- 变更前执行 `caddy validate`，成功后才 reload。

## 13. 首次部署手册

以下命令中的根目录变量不得使用系统 `HOME`。部署者先把占位值替换为本次部署专用的绝对路径：

```bash
export MEASIX_ROOT=/absolute/path/chosen-for-this-deployment
```

### 13.1 前置条件

- NVIDIA DGX Spark，Linux `aarch64` / Go `arm64`；
- 域名已解析到主机；
- 80/443 可访问；
- 已安装并运行 Caddy；
- 已安装 Node LTS、PM2 和 pm2-logrotate；
- `sudo pm2` 可用；
- 主机时间同步正常。

### 13.2 创建用户和目录

```bash
sudo useradd --system --home-dir "$MEASIX_ROOT" --shell /usr/sbin/nologin measix
sudo mkdir -p "$MEASIX_ROOT"/{releases,config,secrets,data/hub,data/relay,logs,backups,staging,run}
sudo chown root:root "$MEASIX_ROOT" "$MEASIX_ROOT"/{releases,config}
sudo chown root:measix "$MEASIX_ROOT/secrets"
sudo chown -R measix:measix "$MEASIX_ROOT"/{data,logs,backups,staging,run}
sudo chmod 0755 "$MEASIX_ROOT" "$MEASIX_ROOT"/{releases,config}
sudo chmod 0750 "$MEASIX_ROOT"/{secrets,data,logs,backups,staging,run}
```

实现部署脚本时必须逐一解析目标路径并确认位于 `MEASIX_ROOT` 内，不对未验证变量执行递归删除或移动。

### 13.3 安装发布包

```bash
sudo mkdir -p "$MEASIX_ROOT/releases/<version>"
sudo tar -xzf measix-core-<version>-linux-arm64.tar.gz -C "$MEASIX_ROOT/releases/<version>"
cd "$MEASIX_ROOT/releases/<version>"
sha256sum -c SHA256SUMS
sudo ln -sfn "$MEASIX_ROOT/releases/<version>" "$MEASIX_ROOT/current"
```

### 13.4 生成秘密

使用系统 CSPRNG 生成：

- `master.key`：精确 32 raw bytes；
- `jwt-ed25519.seed`：精确 32 raw bytes；
- `relay-service.token`：高熵文本 token；
- 首次管理员密码文件。

秘密文件由 root 拥有、`measix` 组只读（`0640`），秘密目录为 `0750 root:measix`；不在 shell 输出内容，不写进 release、Git 或普通日志。

### 13.5 配置

复制 release 中的 ecosystem/Caddy 模板到 `$MEASIX_ROOT/config`，设置：

- `MEASIX_ROOT`；
- external public origin；
- Hub/Relay listener 和内部 URL；
- DB、spool、assets、log、secret 文件路径；
- config version。

### 13.6 初始化和管理员

```bash
sudo -u measix "$MEASIX_ROOT/current/bin/control-hub" migrate \
  --db "$MEASIX_ROOT/data/hub/hub.db"

sudo -u measix "$MEASIX_ROOT/current/bin/control-hub" bootstrap-admin \
  --db "$MEASIX_ROOT/data/hub/hub.db" \
  --master-key-file "$MEASIX_ROOT/secrets/master.key" \
  --jwt-private-key-file "$MEASIX_ROOT/secrets/jwt-ed25519.seed" \
  --password-file "$MEASIX_ROOT/secrets/initial-admin-password" \
  --deployment-name MEASIX \
  --username admin
```

成功后移除首次管理员密码文件。

### 13.7 启动 PM2

```bash
sudo env MEASIX_ROOT="$MEASIX_ROOT" MEASIX_PUBLIC_ORIGIN="$MEASIX_PUBLIC_ORIGIN" \
  pm2 start "$MEASIX_ROOT/config/ecosystem.config.cjs"
sudo pm2 save
sudo pm2 startup
sudo pm2 status
```

`pm2 startup` 输出的 systemd 命令必须按目标主机实际结果执行，不能在文档中硬编码用户 home。

### 13.8 启用 Caddy

```bash
sudo ln -sfn "$MEASIX_ROOT/config/Caddyfile" /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo systemctl reload caddy
```

### 13.9 验收

```bash
curl -fsS https://<domain>/live
curl -fsS https://<domain>/ready
curl -fsS https://<domain>/.well-known/measix
sudo pm2 status
```

再通过 Admin 登录核对 System 四页签，并用当前 Preview Android 完成 Enrollment、Snapshot sync、一次实际启用资源调用和 Usage 查看。

## 14. 升级、回退与恢复手册

### 14.1 升级

1. 解压新 release 到 `releases/<new-version>` 并验证 SHA256；
2. 使用旧版本在线生成 Hub backup；
3. 停止 Hub/Relay；
4. 冷备份 config、secrets、spool 并写入 backup manifest；
5. 使用新 release binary 对 Hub DB 执行 `migrate` 和 `check`；
6. 切换 `current` 软链接；
7. 如模板版本变化，按手册更新 `config/`；
8. `sudo pm2 restart ... --update-env`；
9. 验证 Caddy、readiness、Admin 状态和实际 Runtime 调用；
10. 验收前保留旧 release 和升级前 backup。

### 14.2 回退

- 数据库版本未变化：切回旧 release，重启并验证；
- 数据库已迁移：停止服务，恢复升级前 Hub DB/config/secrets/spool，再切回旧 release；
- 不允许旧 binary 直接打开已经迁移的新 DB。

### 14.3 灾难恢复

在 staging 目录完成恢复验证后才替换活动数据。验证至少包括：

- `control-hub check`；
- Admin 登录；
- active Release/generation/revision；
- Relay control 收敛；
- Usage 查询；
- 一次真实 Runtime 请求；
- Android refresh/sync。

## 15. 实施顺序

### A. 权威和协议

- [x] 更新 Architecture 对 Preview 后 forward migration、Admin 日志/遥测产品边界和验收要求；
- [x] 修复 Android/Portal contract hash 漂移；
- [x] 实现 release manifest、消费端导出校验和 Preview contract gate；
- [x] 更新 Core 协议与发布文档。

### B. 日志和遥测

- [x] 实现 common observability logger/middleware；
- [x] 补齐内部 Preview 最小关键事件和状态转换日志；
- [x] 实现固定 60 分钟 telemetry；
- [x] 扩展 Relay ControlStatus；
- [x] 实现 Admin telemetry/events API；
- [x] 实现 System 页面四页签与有界高效渲染；
- [x] 添加脱敏、边界、性能和 UI 测试。

### C. 持久化

- [x] 将单初始化 SQL 转为 v1 forward migration；
- [x] 实现生产 `migrate`、startup version check 和 schema identity；
- [x] 统一 dev/prod migration owner；
- [x] 扩展 backup metadata/check；
- [x] 实现 Hub/Relay 一致性备份、备份校验和可执行的升级/恢复/回退步骤，并通过自动恢复场景；目标主机演练仍见外部门禁。

### D. 发布部署

- [x] 实现并验证 DGX Spark Linux ARM64 release 构建；
- [x] 实现单根目录 ecosystem/Caddy 模板；
- [x] 实现 SHA256/release manifest；
- [x] 用解压包在无源码目录运行 smoke；
- [x] 完善本文为最终部署手册；
- [ ] 在目标 DGX Spark 上实测首次部署和升级恢复；本地交叉构建或容器 smoke 不能替代主机验收。

### E. 验证和独立审查

- [x] Core Go tests/vet/race（race 在 WSL Linux + GCC 环境）；
- [x] Admin test/typecheck/build/E2E；
- [x] Portal test/typecheck/build；
- [x] contract/generate/drift；
- [x] system/candidate/browser harness；
- [x] migration/backup/restore/package tests；
- [x] Android contract/JVM 和 emulator connected tests；
- [ ] 至少一台物理 Android 设备完成当前候选的 Preview 主流程；
- [ ] Caddy + sudo PM2 + binary package 实际部署验证；
- [x] 独立子代理逐条审查本文；
- [x] 修复独立审查中的代码/手册问题并重跑受影响及最终本地完整验证。

## 16. 最终验收标准

只有以下全部具备当前候选的可核验证据，才称为可部署的 S0.2 Preview：

1. 四仓库 pin 到明确 commit，协议 hash 完全一致；
2. release 包不含源码或秘密，二进制显示真实 buildVersion；
3. 单一 `MEASIX_ROOT` 保存全部 MEASIX-owned 配置、密钥、数据、日志和备份；
4. Caddy 只暴露 HTTPS public origin，internal route 不可达；
5. sudo PM2 能启动、停止、重启和开机恢复 Hub/Relay；
6. Admin 能观察两个业务进程日志、15/60 分钟遥测和既有运行状态；
7. 日志无 forbidden material，列表与图表保持有界；
8. 数据库 forward migration、备份、恢复、迁移后回退均实测；
9. 解压后的 release 在无源码目录通过 smoke；
10. 当前 Android 对实际部署完成 Enrollment、Snapshot v4、Runtime、Usage、重启恢复；
11. 自动门禁、实际部署验收和独立审查均完成，确认问题已修复。
