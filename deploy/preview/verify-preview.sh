#!/usr/bin/env bash
set -euo pipefail

root=${1:-}
if [[ -z "$root" || "$root" != /* || "$root" == / ]]; then
  echo "usage: verify-preview.sh /absolute/measix-root" >&2
  exit 2
fi
root=$(readlink -m -- "$root")
cd "$root/current"
sha256sum -c SHA256SUMS
"$root/current/bin/control-hub" check --db "$root/data/hub/hub.db"
curl -fsS "http://127.0.0.1:9004/live" >/dev/null
curl -fsS "http://127.0.0.1:9004/ready" >/dev/null
curl -fsS "http://127.0.0.1:9004/.well-known/measix" >/dev/null
curl -fsS "http://127.0.0.1:9002/live" >/dev/null
echo "preview verification passed"
