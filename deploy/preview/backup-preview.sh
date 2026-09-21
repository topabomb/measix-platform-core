#!/usr/bin/env bash
set -euo pipefail

root=${1:-}
if [[ -z "$root" || "$root" != /* || "$root" == / ]]; then
  echo "usage: backup-preview.sh /absolute/measix-root" >&2
  exit 2
fi
root=$(readlink -m -- "$root")
for path in "$root/current" "$root/data" "$root/config" "$root/secrets" "$root/backups"; do
  resolved=$(readlink -m -- "$path")
  [[ "$resolved" == "$root"/* ]] || { echo "path escapes MEASIX_ROOT: $path" >&2; exit 2; }
done

stamp=$(date -u +%Y%m%dT%H%M%SZ)
target="$root/backups/$stamp"
install -d -m 0700 -o measix -g measix "$target"
sudo -u measix "$root/current/bin/control-hub" backup --db "$root/data/hub/hub.db" --output "$target/hub.db"
cp -a -- "$root/config" "$target/config"
cp -a -- "$root/secrets" "$target/secrets"
if [[ -f "$root/data/relay/relay-spool.db" ]]; then
  sudo -u measix "$root/current/bin/runtime-relay" backup \
    --spool "$root/data/relay/relay-spool.db" \
    --output "$target/relay-spool.db"
fi
MEASIX_BACKUP_TARGET="$target" MEASIX_BACKUP_CURRENT="$root/current" node -e '
const { createHash } = require("node:crypto");
const { readFileSync, readdirSync, statSync } = require("node:fs");
const { join, relative } = require("node:path");
const target = process.env.MEASIX_BACKUP_TARGET;
const current = process.env.MEASIX_BACKUP_CURRENT;
const release = JSON.parse(readFileSync(join(current, "release.json"), "utf8"));
const metadata = JSON.parse(readFileSync(join(target, "hub.db.metadata.json"), "utf8"));
const files = {};
function visit(dir) {
  for (const name of readdirSync(dir).sort()) {
    const path = join(dir, name); const stat = statSync(path);
    if (stat.isDirectory()) visit(path);
    else if (name !== "backup-manifest.json") {
      const key = relative(target, path).replaceAll("\\", "/");
      files[key] = { sha256: createHash("sha256").update(readFileSync(path)).digest("hex"), size: stat.size, mode: stat.mode & 0o777 };
    }
  }
}
visit(target);
const value = {
  formatVersion: 1, createdAt: new Date().toISOString(),
  release: { version: release.version, source: release.source, manifestSha256: createHash("sha256").update(readFileSync(join(current, "release.json"))).digest("hex"), checksumsSha256: createHash("sha256").update(readFileSync(join(current, "SHA256SUMS"))).digest("hex") },
  configVersion: readFileSync(join(target, "config", "config-version"), "utf8").trim(),
  schema: { identity: metadata.schema, version: metadata.schemaVersion }, files,
};
process.stdout.write(JSON.stringify(value, null, 2) + "\n");
' > "$target/backup-manifest.json"
chmod 0600 "$target/backup-manifest.json"
echo "$target"
