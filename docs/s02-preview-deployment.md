# S0.2 Preview — DGX Spark 部署手册

本手册对应单机 NVIDIA DGX Spark 的内部 Preview。Spark 只运行 MEASIX Hub/Relay，由现有的 root PM2 管理；Spark 不安装 Caddy。另一台已经加入同一 Tailscale 网络的入口服务器负责 TLS、子域名和反向代理，该外部通道不属于本次 Spark 部署验收范围。

## 1. 当前目标环境

2026-09-21 已只读确认：

- Spark：`192.168.100.216`，Ubuntu 24.04.5 LTS，`aarch64`；
- Tailscale 地址：`100.64.0.4`；
- Node `v24.21.0`、npm `11.19.0`、PM2 `7.0.4`；
- `pm2-root.service` 已启用并运行，PM2 home 为 `/root/.pm2`；
- 已有 PM2 服务必须保留，不得执行 `pm2 delete all`、`pm2 stop all` 或覆盖整个 dump；
- `/home/admin/project/service` 已有 `measix-archive`、`deepseek-harness`；
- 9001–9004 当前未占用；
- Spark 未安装 Caddy，这是预期状态；
- systemd 提示 `pm2-root.service` 的磁盘 unit 比已加载版本新，正式部署前经确认执行一次 `sudo systemctl daemon-reload`，但不重启其他服务。

本次固定部署根目录：

```bash
export MEASIX_ROOT=/home/admin/project/service/measix-core
export MEASIX_VERSION=<approved-preview-version>
export MEASIX_PUBLIC_ORIGIN=https://<approved-subdomain>
```

`MEASIX_PUBLIC_ORIGIN` 是 Android/Portal 使用的远端 Caddy HTTPS 地址，不是 Spark 的监听地址。

## 2. 网络边界

端口固定，不提供可配置绑定地址：

| 端口 | 监听 | 用途 |
| --- | --- | --- |
| 9001 | `127.0.0.1` | Hub internal，仅 Relay 本机访问 |
| 9002 | `0.0.0.0` | Relay Runtime，供局域网/Tailscale Caddy 访问 |
| 9003 | `127.0.0.1` | Relay internal，仅 Hub 本机访问 |
| 9004 | `0.0.0.0` | Hub/Admin/Client，供局域网/Tailscale Caddy 访问 |

9001/9003 不得通过防火墙、Tailscale funnel 或反向代理暴露。9002/9004 只应由受信局域网/Tailscale ACL 访问。

## 3. 主目录布局

```text
/home/admin/project/service/measix-core/
├── run.sh                         # PM2 唯一进程入口：run.sh hub|relay
├── ecosystem.config.cjs           # root PM2 配置
├── current -> releases/<version>/
├── releases/<version>/
│   ├── bin/{control-hub,runtime-relay}
│   ├── assets/{admin,portal}/
│   ├── deploy/
│   ├── release.json
│   └── SHA256SUMS
├── config/
│   ├── config-version
│   └── public-origin
├── secrets/
├── data/{hub,relay}/
├── logs/
├── backups/
├── staging/
└── run/
```

发布、升级和回退不得删除或覆盖 `config/`、`secrets/`、`data/`、`logs/`、`backups/`。`run.sh` 和 `ecosystem.config.cjs` 是 release-owned 控制文件，升级时从已验证的新 release 刷新。

## 4. 发布包生成与检查

在开发机的 Core 仓库执行：

```text
node scripts/build-preview-release.mjs <version>
```

发布门禁会固定 Architecture/Core/Portal/Android commit、协议 hash、Snapshot v4、Portal Bridge v3 和 Enrollment v1，并输出：

```text
.artifacts/releases/measix-core-<version>-linux-arm64.tar.gz
```

归档只包含 Linux ARM64 二进制、静态资源、部署文件、`release.json` 和 `SHA256SUMS`，不包含源码、数据库、日志或秘密。

## 5. 正式部署前检查（只读）

```bash
uname -m
node --version
sudo pm2 status
systemctl is-active pm2-root
tailscale ip -4
ss -lnt | grep -E ':(9001|9002|9003|9004)\b' || true
df -h /home/admin/project/service
```

必须确认架构为 `aarch64`、PM2 现有服务仍 online、9001–9004 空闲，并记录部署前的 `sudo pm2 status`。

## 6. 首次部署（必须取得用户确认后执行）

以下步骤当前尚未在 Spark 执行。

1. 在主目录内创建 staging/release 目录并上传归档；不得把源码部署到 Spark。
2. 解压后先运行 `sha256sum -c SHA256SUMS`。
3. 执行首次安装器：

```bash
sudo "$MEASIX_ROOT/releases/$MEASIX_VERSION/deploy/install-preview.sh" \
  "$MEASIX_ROOT" \
  "$MEASIX_ROOT/releases/$MEASIX_VERSION" \
  "$MEASIX_PUBLIC_ORIGIN"
```

安装器会：

- 创建 `measix` 系统用户及单根目录布局；
- 安装根目录 `run.sh` 和 `ecosystem.config.cjs`；
- 生成密钥、迁移数据库、创建首个管理员；
- 通过 root PM2 增加且只增加 `measix-relay`、`measix-hub`；
- 执行 `pm2 save`，保留现有 PM2 应用。

安装完成后配置日志轮转（如果 root PM2 尚未安装该模块）：

```bash
sudo pm2 install pm2-logrotate
sudo pm2 set pm2-logrotate:max_size 20M
sudo pm2 set pm2-logrotate:retain 7
sudo pm2 set pm2-logrotate:compress true
sudo pm2 set pm2-logrotate:rotateInterval '0 0 * * *'
sudo pm2 save
```

首次登录后删除且只删除：

```bash
sudo rm -- "$MEASIX_ROOT/secrets/initial-admin-password"
```

## 7. Spark 本机/局域网验收

本批不从远端 Caddy 或公网验证，只验证 Spark 服务本身：

```bash
sudo pm2 status
sudo "$MEASIX_ROOT/current/deploy/verify-preview.sh" "$MEASIX_ROOT"
curl -fsS http://127.0.0.1:9004/live
curl -fsS http://127.0.0.1:9004/ready
curl -fsS http://127.0.0.1:9002/live
sudo ss -lntp | grep -E ':(9001|9002|9003|9004)\b'
```

预期：9002/9004 为 `0.0.0.0`，9001/9003 为 `127.0.0.1`；两个新 PM2 app online，原有 PM2 app 状态不变。

Admin 登录后检查 System 四页签、当前 release、Relay ready、配置 revision、spool、近期遥测和事件。Android 模拟器验证视为本批 Android 验收。

## 8. 远端 Caddy 交接（不在 Spark 执行）

发布包中的 `deploy/Caddyfile.template` 只是远端入口服务器的参考片段，不由 Spark 安装器复制或 reload。入口服务器应将 `__MEASIX_SPARK_TAILSCALE_IP__` 替换为 `100.64.0.4`，将 `__MEASIX_PUBLIC_ORIGIN__` 替换为获批子域名：

```caddyfile
https://<approved-subdomain> {
	@private path /internal /internal/*
	handle @private {
		respond 404
	}

	@runtime path /runtime/v1 /runtime/v1/*
	handle @runtime {
		reverse_proxy 100.64.0.4:9002
	}

	handle {
		reverse_proxy 100.64.0.4:9004
	}
}
```

远端 DNS、TLS、Caddy reload 和公网连通性由入口服务器维护者单独验收，本任务不操作也不验证该服务器。

## 9. 备份

```bash
backup=$(sudo "$MEASIX_ROOT/current/deploy/backup-preview.sh" "$MEASIX_ROOT")
sudo "$MEASIX_ROOT/current/deploy/verify-backup.sh" "$backup" "$MEASIX_ROOT/current"
```

备份包含 Hub/Relay 一致性 SQLite 镜像、config、secrets 和逐文件 hash manifest。验证后必须另存一份到 Spark 之外的受保护存储。

## 10. 升级

1. 解压新 release 并校验 SHA256；
2. 使用旧 release 创建并验证备份；
3. `sudo pm2 stop measix-hub measix-relay`，不得停止其他 PM2 app；
4. 用新 binary 执行 `migrate` 和 `check`；
5. 从新 release 安装新的 `$MEASIX_ROOT/run.sh` 与 `$MEASIX_ROOT/ecosystem.config.cjs`；
6. 原子切换 `current`；
7. `sudo pm2 startOrReload "$MEASIX_ROOT/ecosystem.config.cjs"`，然后 `sudo pm2 save`；
8. 执行第 7 节完整验收，并保留旧 release/备份直到确认完成。

`config/public-origin` 属于持久配置，升级不得被模板覆盖。新增 optional 配置不提升 config version；required 配置或语义改变必须提供明确转换步骤。

## 11. 回退与恢复

数据库版本未变化时，可以切回旧 release、刷新旧版 `run.sh`/ecosystem 并只重启两个 MEASIX app。数据库已经迁移时，旧 binary 不得打开新数据库，必须停止两个 app，验证升级前备份，再同时恢复 Hub DB、可选 Relay spool、config、secrets 和对应 release。

恢复一律先进入 `$MEASIX_ROOT/staging/recovery-<timestamp>`；当前数据移入 `original/`，不得直接删除。候选数据库先执行 `control-hub check`，验收前保留 `original/`。

## 12. 日志与排障

- Hub：`$MEASIX_ROOT/logs/hub.jsonl`、`hub.stderr.log`；
- Relay：`$MEASIX_ROOT/logs/relay.jsonl`、`relay.stderr.log`；
- `sudo pm2 status` / `sudo pm2 describe measix-hub`；
- Admin System 页读取有界的近期结构化事件和 15/60 分钟遥测；
- 未知错误只显示稳定安全错误码，不输出请求 body、token、prompt 或密钥。

Spark 没有本机 Caddy，因此 Caddy 日志、TLS 与外部 502 应在远端入口服务器排查。
