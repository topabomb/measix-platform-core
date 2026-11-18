#!/usr/bin/env bash
set -euo pipefail

service_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
role=${1:-}

case "$role" in
  relay)
    exec "$service_root/current/bin/runtime-relay" \
      --public-listen 0.0.0.0:9002 \
      --internal-listen 127.0.0.1:9003 \
      --spool "$service_root/data/relay/relay-spool.db" \
      --hub-internal-url http://127.0.0.1:9001 \
      --hub-service-token-file "$service_root/secrets/relay-service.token"
    ;;
  hub)
    export HUB_LISTEN_ADDR=0.0.0.0:9004
    export HUB_INTERNAL_LISTEN_ADDR=127.0.0.1:9001
    export HUB_DB_PATH="$service_root/data/hub/hub.db"
    export HUB_MASTER_KEY_FILE="$service_root/secrets/master.key"
    export HUB_JWT_PRIVATE_KEY_FILE="$service_root/secrets/jwt-ed25519.seed"
    export HUB_RELAY_SERVICE_TOKEN_FILE="$service_root/secrets/relay-service.token"
    export RELAY_INTERNAL_URL=http://127.0.0.1:9003
    export HUB_ADMIN_ASSETS_DIR="$service_root/current/assets/admin"
    export HUB_PORTAL_ASSETS_DIR="$service_root/current/assets/portal"
    export HUB_DIAGNOSTICS_LOG_DIR="$service_root/logs"
    export HUB_PUBLIC_ORIGIN="$(<"$service_root/config/public-origin")"
    exec "$service_root/current/bin/control-hub" run
    ;;
  *)
    echo "usage: $0 hub|relay" >&2
    exit 2
    ;;
esac
