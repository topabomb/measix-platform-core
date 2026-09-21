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
if [[ -f "$root/data/relay/relay-spool.db" ]]; then cp -a -- "$root/data/relay/relay-spool.db" "$target/relay-spool.db"; fi
release=$(readlink -f -- "$root/current")
MEASIX_BACKUP_RELEASE="$release" MEASIX_BACKUP_ROOT="$root" node -e '
const value = {createdAt: new Date().toISOString(), release: process.env.MEASIX_BACKUP_RELEASE, root: process.env.MEASIX_BACKUP_ROOT};
process.stdout.write(JSON.stringify(value) + "\n");
' > "$target/backup-manifest.json"
chmod 0600 "$target/backup-manifest.json"
echo "$target"
