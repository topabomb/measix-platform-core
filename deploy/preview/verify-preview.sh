#!/usr/bin/env bash
set -euo pipefail

root=${1:-}
origin=${2:-}
if [[ -z "$root" || "$root" != /* || "$root" == / || -z "$origin" ]]; then
  echo "usage: verify-preview.sh /absolute/measix-root https://core.example.com" >&2
  exit 2
fi
root=$(readlink -m -- "$root")
cd "$root/current"
sha256sum -c SHA256SUMS
"$root/current/bin/control-hub" check --db "$root/data/hub/hub.db"
curl -fsS "$origin/live" >/dev/null
curl -fsS "$origin/ready" >/dev/null
curl -fsS "$origin/.well-known/measix" >/dev/null
echo "preview verification passed"
