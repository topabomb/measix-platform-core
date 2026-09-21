# S0.2 Preview — DGX Spark 部署手册

本手册对应单机 NVIDIA DGX Spark 的内部 Preview。Spark 只运行 MEASIX Hub/Relay，由现有的 root PM2 管理；Spark 不安装 Caddy。另一台已经加入同一 Tailscale 网络的入口服务器负责 TLS、子域名和反向代理，并由入口维护者独立配置和验收。

## 1. 当前目标环境

目标机实际 LAN/Tailscale 地址、主机名和 Public Origin 属于部署现场信息，不写入 Git；部署后记录在服务主目录中权限为 `0640 root:admin` 的 `deployment-local.md`。已确认的通用条件：

- Spark 运行 Ubuntu 24.04 LTS，架构为 `aarch64`；
- Spark 和远端 Caddy 入口已加入同一 Tailscale 网络；
- Node `v24.21.0`、npm `11.19.0`、PM2 `7.0.4`；
- `pm2-root.service` 已启用并运行，PM2 home 为 `/root/.pm2`；
- 已有 PM2 服务必须保留，不得执行 `pm2 delete all`、`pm2 stop all` 或覆盖整个 dump；
- 服务父目录已有其他项目，MEASIX 必须使用独立子目录；
- 首次部署前必须确认 9001–9004 未占用；
- Spark 未安装 Caddy，这是预期状态；
- 如 systemd 提示 PM2 unit 的磁盘版本变化，只执行 `sudo systemctl daemon-reload`，不重启其他服务。

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
├── deployment-local.md            # 现场地址和验收记录；私有、不得提交 Git
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

1. 在主目录内创建 staging/release 目录并上传归档；不得把源码部署到 Spark。
2. 解压后先运行 `sha256sum -c SHA256SUMS`。
3. 确认 `measix` 用户能穿越部署路径的所有父目录。若 `namei -l` 显示父目录阻断，只给阻断父目录增加 execute-only ACL，不开放目录读取：

```bash
namei -l "$MEASIX_ROOT/releases/$MEASIX_VERSION/bin/control-hub"
sudo setfacl -m u:measix:--x <blocking-parent-directory>
```

4. 执行首次安装器：

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

安装器把一次性初始密码写入 root-only 文件。只在受控终端查看，将它保存到团队密码库并完成一次 Admin 登录；不得写入部署记录、工单或 Git：

```bash
sudo cat "$MEASIX_ROOT/secrets/initial-admin-password"
```

确认密码库中的凭据可以重新登录后，删除且只删除：

```bash
sudo rm -- "$MEASIX_ROOT/secrets/initial-admin-password"
```

密码文件删除后无法从数据库反查明文。凭据遗失时，使用 `control-hub bootstrap-admin --add-admin` 和新的受保护 password file 新增具名管理员；不要复用 SSH 密码，也不要为找回旧密码修改数据库。

## 7. Spark 本机/局域网验收

先验证 Spark 服务本身；不要把公网连通误当作进程和数据健康：

```bash
sudo pm2 status
sudo "$MEASIX_ROOT/current/deploy/verify-preview.sh" "$MEASIX_ROOT"
curl -fsS http://127.0.0.1:9004/live
curl -fsS http://127.0.0.1:9004/ready
curl -fsS http://127.0.0.1:9002/live
sudo ss -lntp | grep -E ':(9001|9002|9003|9004)\b'
```

预期：9002/9004 为 `0.0.0.0`，9001/9003 为 `127.0.0.1`；两个新 PM2 app online，原有 PM2 app 状态不变。尚未发布 Managed Configuration 时 Relay `/ready` 返回 503 是预期状态；发布并收敛后必须为 200。

Admin 登录后检查 System 四页签、当前 release、Relay ready、配置 revision、spool、近期遥测和事件。Android 模拟器验证视为本批 Android 验收。

## 8. 远端 Caddy 交接与公共入口验收（不在 Spark 执行）

发布包中的 `deploy/Caddyfile.template` 只是远端入口服务器的参考片段，不由 Spark 安装器复制或 reload。入口服务器应从私有 `deployment-local.md` 取得现场值，并替换模板中的 `__MEASIX_SPARK_TAILSCALE_IP__` 与 `__MEASIX_PUBLIC_ORIGIN__`：

```caddyfile
https://<approved-subdomain> {
	@private path /internal /internal/*
	handle @private {
		respond 404
	}

	@runtime path /runtime/v1 /runtime/v1/*
	handle @runtime {
		reverse_proxy <SPARK_TAILSCALE_IP>:9002
	}

	handle {
		reverse_proxy <SPARK_TAILSCALE_IP>:9004
	}
}
```

远端 DNS、证书和 Caddy reload 由入口服务器维护者执行。对方确认完成后，从普通客户端只读验收公共入口；现场值仍只记入私有 `deployment-local.md`：

```bash
curl -fsS https://<approved-subdomain>/.well-known/measix
curl -fsS https://<approved-subdomain>/live
curl -fsS https://<approved-subdomain>/ready
curl -fsS https://<approved-subdomain>/admin/ >/dev/null
curl -fsS https://<approved-subdomain>/portal/ >/dev/null
test "$(curl -sS -o /dev/null -w '%{http_code}' https://<approved-subdomain>/internal)" = 404
test "$(curl -sS -o /dev/null -w '%{http_code}' https://<approved-subdomain>/internal/test)" = 404
```

最后从公共 origin 发起一次 Runtime 请求。即使业务返回 4xx/5xx，也要先区分“请求已经到达 Relay”与“上游或发布配置不可用”；只有已发布配置下的真实业务成功才算端到端通过。

## 9. 发布后的最小业务配置

内部 Preview 只配置实际可用的资源，不为凑齐类型发布失效占位项：

1. 在 Admin 创建上游 Secret、上游连接，执行 Test 后 Apply；Secret 值不写入文档或日志。
2. 在企业配置中至少建立一个真实模型、必要的系统 TTS 或 MCP、一个默认助手和一个 Starter；仅为实际可用的能力设置默认值。
3. 保存草稿，依次执行 Validate、Snapshot Preview、Review、Publish，并等待 Activation `COMPLETED`。
4. 确认 Relay `/ready` 为 200，Admin System 显示 active generation 与 control revision 已收敛。
5. 创建内部成员和一小时一次性接入资料，在 Android 模拟器粘贴或扫码接入；接入资料属于凭据，不写入 Git 或共享日志。
6. Android 完成同步、默认助手真实请求、Usage 回查和应用重启恢复后，才把该发布标为“可用”。

上游地址、资源名称、模型 key、用户和现场验证结果记录在私有 `deployment-local.md`。Admin 对 token、字符、秒、请求数和图片数使用统一的人类可读格式；详情保留精确整数，额度模板和单用户额度允许用 `100k`、`2M` 这类简写输入并在保存时转换为精确整数，`LEVEL_0` 的未知 token/费用不得显示成零。

## 10. 备份

```bash
backup=$(sudo "$MEASIX_ROOT/current/deploy/backup-preview.sh" "$MEASIX_ROOT")
sudo "$MEASIX_ROOT/current/deploy/verify-backup.sh" "$backup" "$MEASIX_ROOT/current"
```

备份包含 Hub/Relay 一致性 SQLite 镜像、config、secrets 和逐文件 hash manifest。验证后必须另存一份到 Spark 之外的受保护存储。

## 11. 升级

1. 解压新 release 并校验 SHA256；
2. 使用旧 release 创建并验证备份；
3. `sudo pm2 stop measix-hub measix-relay`，不得停止其他 PM2 app；
4. 用新 binary 执行 `migrate` 和 `check`；
5. 从新 release 安装新的 `$MEASIX_ROOT/run.sh` 与 `$MEASIX_ROOT/ecosystem.config.cjs`；
6. 原子切换 `current`；
7. `sudo pm2 startOrReload "$MEASIX_ROOT/ecosystem.config.cjs"`，然后 `sudo pm2 save`；
8. 执行第 7–9 节验收；确认完成后删除 staging 和旧 release，只保留当前 release 与最近一次已验证数据备份。

`config/public-origin` 属于持久配置，升级不得被模板覆盖。新增 optional 配置不提升 config version；required 配置或语义改变必须提供明确转换步骤。

## 12. 数据恢复（首发不保留旧版回退）

首次 Preview 不保留多版本二进制回退。故障恢复使用当前 release 与已验证备份，同时恢复 Hub DB、可选 Relay spool、config 和 secrets；不得只恢复数据库而继续使用不匹配的配置或密钥。后续版本若需要旧版回退，必须在对应升级说明中明确数据库兼容边界后再启用。

恢复一律先进入 `$MEASIX_ROOT/staging/recovery-<timestamp>`；当前数据移入 `original/`，不得直接删除。候选数据库先执行 `control-hub check`，验收前保留 `original/`。

## 13. 日志与排障

- Hub：`$MEASIX_ROOT/logs/hub.jsonl`、`hub.stderr.log`；
- Relay：`$MEASIX_ROOT/logs/relay.jsonl`、`relay.stderr.log`；
- `sudo pm2 status` / `sudo pm2 describe measix-hub`；
- Admin System 页读取有界的近期结构化事件和 15/60 分钟遥测；
- 未知错误只显示稳定安全错误码，不输出请求 body、token、prompt 或密钥。

Spark 没有本机 Caddy，因此 Caddy 日志、TLS 与外部 502 应在远端入口服务器排查。
