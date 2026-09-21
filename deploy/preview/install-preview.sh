#!/usr/bin/env bash
set -euo pipefail

root=${1:-}
release_dir=${2:-}
public_origin=${3:-}
if [[ $(id -u) -ne 0 ]]; then echo "install-preview.sh must run as root" >&2; exit 2; fi
if [[ -z "$root" || "$root" != /* || "$root" == / ]]; then echo "MEASIX_ROOT must be an explicit absolute non-root path" >&2; exit 2; fi
if [[ -z "$release_dir" || "$release_dir" != /* ]]; then echo "release directory must be absolute" >&2; exit 2; fi
if [[ ! "$public_origin" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]]; then echo "public origin must be an HTTPS origin without a path" >&2; exit 2; fi

root=$(readlink -m -- "$root")
release_dir=$(readlink -f -- "$release_dir")
[[ "$release_dir" == "$root"/releases/* ]] || { echo "release must be below MEASIX_ROOT/releases" >&2; exit 2; }
[[ -f "$release_dir/release.json" && -x "$release_dir/bin/control-hub" && -x "$release_dir/bin/runtime-relay" ]] || { echo "invalid release directory" >&2; exit 2; }
[[ ! -e "$root/config/config-version" && ! -e "$root/data/hub/hub.db" ]] || { echo "deployment already initialized; use the upgrade runbook" >&2; exit 2; }
(cd "$release_dir" && sha256sum -c SHA256SUMS)

if ! id measix >/dev/null 2>&1; then useradd --system --home-dir "$root" --shell /usr/sbin/nologin measix; fi
if ! sudo -u measix test -x "$release_dir/bin/control-hub"; then
  echo "measix cannot traverse the selected deployment path; grant execute-only access on the blocking parent directory and retry" >&2
  echo "inspect with: namei -l $release_dir/bin/control-hub" >&2
  exit 2
fi
install -d -m 0755 -o root -g root "$root" "$root/releases" "$root/config"
install -d -m 0750 -o root -g measix "$root/secrets"
chmod 0755 "$release_dir"
for dir in data data/hub data/relay logs backups staging run; do install -d -m 0750 -o measix -g measix "$root/$dir"; done
ln -sfn -- "$release_dir" "$root/current"
install -m 0644 -o root -g root "$release_dir/deploy/ecosystem.config.cjs" "$root/ecosystem.config.cjs"
install -m 0755 -o root -g root "$release_dir/deploy/run.sh" "$root/run.sh"
printf '1\n' > "$root/config/config-version"
printf '%s\n' "$public_origin" > "$root/config/public-origin"
chown root:measix "$root/config/config-version" "$root/config/public-origin"
chmod 0640 "$root/config/config-version" "$root/config/public-origin"
node -e 'require(process.argv[1])' "$root/ecosystem.config.cjs"

umask 077
head -c 32 /dev/urandom > "$root/secrets/master.key"
head -c 32 /dev/urandom > "$root/secrets/jwt-ed25519.seed"
head -c 48 /dev/urandom | base64 | tr -d '\n' > "$root/secrets/relay-service.token"
printf '\n' >> "$root/secrets/relay-service.token"
head -c 24 /dev/urandom | base64 | tr -d '\n' > "$root/secrets/initial-admin-password"
printf '\n' >> "$root/secrets/initial-admin-password"
chown root:measix "$root/secrets"/*
chmod 0640 "$root/secrets"/*

sudo -u measix "$root/current/bin/control-hub" migrate --db "$root/data/hub/hub.db"
sudo -u measix "$root/current/bin/control-hub" bootstrap-admin \
  --db "$root/data/hub/hub.db" \
  --master-key-file "$root/secrets/master.key" \
  --jwt-private-key-file "$root/secrets/jwt-ed25519.seed" \
  --password-file "$root/secrets/initial-admin-password" \
  --deployment-name MEASIX --username admin

pm2 start "$root/ecosystem.config.cjs"
pm2 save
echo "Initial password remains at $root/secrets/initial-admin-password; remove it after the first successful login."
