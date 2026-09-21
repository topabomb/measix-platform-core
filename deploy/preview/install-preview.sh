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
install -d -m 0755 -o root -g root "$root" "$root/releases" "$root/config"
install -d -m 0750 -o root -g measix "$root/secrets"
chmod 0755 "$release_dir"
for dir in data data/hub data/relay logs backups staging run; do install -d -m 0750 -o measix -g measix "$root/$dir"; done
ln -sfn -- "$release_dir" "$root/current"
install -m 0640 -o root -g measix "$release_dir/deploy/ecosystem.config.cjs" "$root/config/ecosystem.config.cjs"
printf '1\n' > "$root/config/config-version"
chmod 0640 "$root/config/config-version"
escaped_origin=${public_origin//&/\\&}
sed "s|__MEASIX_PUBLIC_ORIGIN__|$escaped_origin|g" "$release_dir/deploy/Caddyfile.template" > "$root/config/Caddyfile.tmp"
install -m 0644 -o root -g root "$root/config/Caddyfile.tmp" "$root/config/Caddyfile"
rm -f -- "$root/config/Caddyfile.tmp"
caddy validate --config "$root/config/Caddyfile" --adapter caddyfile

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

env MEASIX_ROOT="$root" MEASIX_PUBLIC_ORIGIN="$public_origin" pm2 start "$root/config/ecosystem.config.cjs" --env production
pm2 save
ln -sfn -- "$root/config/Caddyfile" /etc/caddy/Caddyfile
systemctl reload caddy
echo "Initial password remains at $root/secrets/initial-admin-password; remove it after the first successful login."
