# S0.2 Preview deployment on NVIDIA DGX Spark

This is the operator runbook for the internal Preview. The target is one NVIDIA DGX Spark running Linux `aarch64`. Caddy terminates TLS and reverse-proxies the public origin. Root-owned PM2 manages the Hub and Relay lifecycle. The application is installed from an ARM64 binary release archive; Go, pnpm and source code are not required on the server.

## 1. Required deployment inputs

Choose and record:

```bash
export MEASIX_ROOT=/absolute/path/chosen-for-this-deployment
export MEASIX_PUBLIC_ORIGIN=https://core.example.com
export MEASIX_VERSION=0.2.0-preview.1
```

`MEASIX_ROOT` has no default. It must be an absolute standalone directory and must not be `/`. All MEASIX-owned releases, configuration, secrets, databases, logs, backups, staging and runtime files stay below it. Only the system installations and daemon metadata of Caddy and PM2, plus `/etc/caddy/Caddyfile`, are outside this tree.

The public origin must already resolve to the DGX host. Inbound TCP 80/443 must be allowed; ports 9001–9004 remain loopback-only.

Required host software:

- Linux `uname -m` reports `aarch64`;
- time synchronization is healthy;
- Caddy is installed and managed by systemd;
- Node LTS, PM2 and `pm2-logrotate` are installed for root;
- `sudo pm2 status` works;
- `tar`, `sha256sum`, `curl`, `base64`, `sed` and `sudo` are available.

## 2. Build the release on a trusted build host

Core and Enterprise Portal worktrees must be committed and clean. From Core:

```text
node scripts/build-preview-release.mjs 0.2.0-preview.1
```

The builder regenerates contracts and rejects drift, verifies the pinned S0.2/v4 Preview protocol baseline, builds the Admin and Portal production assets, cross-compiles static Linux ARM64 Hub/Relay binaries, and creates:

```text
.artifacts/releases/measix-core-0.2.0-preview.1-linux-arm64.tar.gz
```

The archive contains only `bin/`, `assets/`, `deploy/`, `release.json` and `SHA256SUMS`. It contains no source, Git metadata, `node_modules`, database, logs or secrets.

Transfer the archive to the DGX through the deployment channel. Do not unpack it over an existing release.

## 3. Stage and verify the archive

```bash
sudo install -d -m 0755 -o root -g root "$MEASIX_ROOT/releases/$MEASIX_VERSION"
sudo tar -xzf "measix-core-$MEASIX_VERSION-linux-arm64.tar.gz" \
  -C "$MEASIX_ROOT/releases/$MEASIX_VERSION"
cd "$MEASIX_ROOT/releases/$MEASIX_VERSION"
sha256sum -c SHA256SUMS
file bin/control-hub bin/runtime-relay
```

Both binaries must report Linux ARM64/aarch64. Inspect `release.json` and confirm version, Core/Portal commits, target and protocol hashes.

## 4. First installation

The packaged installer is first-install only and refuses an existing config version or Hub database. It validates that the release is below the selected root, creates the `measix` service user and directory tree, generates secrets without printing them, initializes schema v1, bootstraps the first administrator, starts PM2 and validates Caddy before reload.

```bash
sudo "$MEASIX_ROOT/releases/$MEASIX_VERSION/deploy/install-preview.sh" \
  "$MEASIX_ROOT" \
  "$MEASIX_ROOT/releases/$MEASIX_VERSION" \
  "$MEASIX_PUBLIC_ORIGIN"
```

Then enable PM2 boot persistence. Run the exact systemd command printed by:

```bash
sudo pm2 startup
sudo pm2 save
```

Do not copy a hard-coded PM2 home from another host.

Configure bounded log rotation:

```bash
sudo pm2 install pm2-logrotate
sudo pm2 set pm2-logrotate:max_size 20M
sudo pm2 set pm2-logrotate:retain 7
sudo pm2 set pm2-logrotate:compress true
sudo pm2 set pm2-logrotate:rotateInterval '0 0 * * *'
sudo pm2 save
```

The initial password is stored at `$MEASIX_ROOT/secrets/initial-admin-password`. Read it only through a protected administrator session, complete the first login, then remove that one file. The remaining key/token files are required for service operation and recovery.

## 5. First acceptance

```bash
sudo pm2 status
sudo systemctl status caddy --no-pager
curl -fsS "$MEASIX_PUBLIC_ORIGIN/live"
curl -fsS "$MEASIX_PUBLIC_ORIGIN/ready"
curl -fsS "$MEASIX_PUBLIC_ORIGIN/.well-known/measix"
sudo "$MEASIX_ROOT/current/deploy/verify-preview.sh" "$MEASIX_ROOT" "$MEASIX_PUBLIC_ORIGIN"
```

In Admin, verify all four System tabs:

1. Hub/Relay builds, database health, public origin and Portal;
2. desired/applied revision, bundle and activation state;
3. 15/60-minute telemetry, spool and Usage diagnostics;
4. bounded recent Hub/Relay events with no credentials or personal data.

Then use the Preview Android client for enrollment, Snapshot sync, one real configured resource request and Usage visibility. HTTP readiness alone is not runtime convergence or device acceptance.

## 6. Backup

Create a local recovery set before every upgrade and periodically during Preview use:

```bash
sudo "$MEASIX_ROOT/current/deploy/backup-preview.sh" "$MEASIX_ROOT"
```

The timestamped directory contains a consistent checked Hub backup and metadata, configuration, secrets, optional Relay spool and a manifest naming the active release. Copy a recovery set to protected storage outside the DGX. A backup left only on the same disk is not disaster recovery.

## 7. Upgrade

1. Stage the new archive under a new `$MEASIX_ROOT/releases/<version>` directory and verify `SHA256SUMS` and `release.json`.
2. Run the old release's `backup-preview.sh` and copy the result off-host.
3. Stop both processes: `sudo pm2 stop measix-hub measix-relay`.
4. Preserve the current symlink target: `readlink -f "$MEASIX_ROOT/current"`.
5. Run the new binary against the persistent DB:

```bash
sudo -u measix "$MEASIX_ROOT/releases/<version>/bin/control-hub" migrate \
  --db "$MEASIX_ROOT/data/hub/hub.db"
sudo -u measix "$MEASIX_ROOT/releases/<version>/bin/control-hub" check \
  --db "$MEASIX_ROOT/data/hub/hub.db"
```

6. If `config-version` changed, apply only the documented transformation. Optional additions do not require a version bump.
7. Atomically switch `current`, then restart with the explicit environment:

```bash
sudo ln -sfn "$MEASIX_ROOT/releases/<version>" "$MEASIX_ROOT/current"
sudo env MEASIX_ROOT="$MEASIX_ROOT" MEASIX_PUBLIC_ORIGIN="$MEASIX_PUBLIC_ORIGIN" \
  pm2 restart "$MEASIX_ROOT/config/ecosystem.config.cjs" --update-env
sudo pm2 save
```

8. Run the complete acceptance in section 5. Retain the previous release and backup until the Preview is accepted.

Migration files and `schema_migrations` rows are immutable. Never repair an upgrade by editing migration history.

## 8. Rollback and restore

If no database migration ran, switch `current` back to the previous release and restart. If a migration ran, the previous binary must not open the upgraded database: restore the pre-upgrade data and release together.

For a restore:

1. stop Hub and Relay;
2. create `$MEASIX_ROOT/staging/recovery-<timestamp>/{original,candidate}`;
3. move the current Hub DB and SQLite sidecars into `original`—do not delete them;
4. copy the backup Hub DB into `candidate`, leaving the backup itself unchanged;
5. use the release recorded by `backup-manifest.json` to run `control-hub check` on the candidate;
6. verify config version, schema version, release identity and the presence/permissions of every required secret;
7. move the checked candidate into `data/hub/hub.db`; for a full point-in-time restore, also stage and replace the Relay spool;
8. switch `current` to the recorded release, start Relay and Hub, and run section 5 plus Admin login, active Release/generation/revision, Usage, one Runtime request and Android refresh/sync;
9. retain `original` until acceptance.

All resolved recovery paths must remain below the explicit `MEASIX_ROOT`. Never use an empty variable, `/`, a home directory or a glob as a recursive move/delete target.

## 9. Routine diagnostics

- `sudo pm2 status` shows lifecycle, not application readiness.
- Admin System is the primary bounded view for process events and recent request telemetry.
- Raw files are `$MEASIX_ROOT/logs/{hub,relay}.jsonl` and matching `.stderr.log`; PM2 manager/Caddy logs are intentionally outside Admin.
- `control-hub check` verifies migration history/checksums, SQLite integrity/foreign keys and required current tables/columns.
- On failure, preserve logs, database, spool, release manifest and backup metadata before restarting. Never delete the spool or database as routine recovery.
