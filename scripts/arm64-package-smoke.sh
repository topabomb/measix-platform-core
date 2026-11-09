#!/usr/bin/env bash
set -euo pipefail

stage=${1:-}
if [[ -z "$stage" || ! -x "$stage/bin/control-hub" || ! -x "$stage/bin/runtime-relay" ]]; then
  echo "usage: arm64-package-smoke.sh /absolute/extracted-release" >&2
  exit 2
fi

task_tmp=$(mktemp -d)
hub_pid=
relay_pid=
cleanup() {
  if [[ -n "$hub_pid" ]]; then kill -TERM "$hub_pid" 2>/dev/null || true; fi
  if [[ -n "$relay_pid" ]]; then kill -TERM "$relay_pid" 2>/dev/null || true; fi
  if [[ -n "$hub_pid" ]]; then wait "$hub_pid" 2>/dev/null || true; fi
  if [[ -n "$relay_pid" ]]; then wait "$relay_pid" 2>/dev/null || true; fi
  rm -rf -- "$task_tmp"
}
trap cleanup EXIT

cd "$task_tmp"
apt download qemu-user-static >/dev/null
dpkg-deb -x qemu-user-static_*.deb qemu
qemu_bin="$task_tmp/qemu/usr/bin/qemu-aarch64-static"
"$qemu_bin" --version | head -1

"$qemu_bin" "$stage/bin/control-hub" migrate --db "$task_tmp/hub.db"
head -c 32 /dev/urandom > "$task_tmp/master.key"
head -c 32 /dev/urandom > "$task_tmp/jwt.seed"
head -c 48 /dev/urandom | base64 | tr -d '\n' > "$task_tmp/relay.token"
printf '\n' >> "$task_tmp/relay.token"
printf 'preview-test-password\n' > "$task_tmp/admin.password"
"$qemu_bin" "$stage/bin/control-hub" bootstrap-admin \
  --db "$task_tmp/hub.db" \
  --master-key-file "$task_tmp/master.key" \
  --jwt-private-key-file "$task_tmp/jwt.seed" \
  --password-file "$task_tmp/admin.password" \
  --deployment-name MEASIX-SMOKE --username admin >/dev/null

"$qemu_bin" "$stage/bin/runtime-relay" \
  --public-listen 127.0.0.1:19002 --internal-listen 127.0.0.1:19003 \
  --spool "$task_tmp/relay.db" \
  --hub-internal-url http://127.0.0.1:19001 \
  --hub-service-token-file "$task_tmp/relay.token" \
  > "$task_tmp/relay.jsonl" 2> "$task_tmp/relay.stderr.log" &
relay_pid=$!

"$qemu_bin" "$stage/bin/control-hub" run \
  --listen 127.0.0.1:19004 --internal-listen 127.0.0.1:19001 \
  --db "$task_tmp/hub.db" \
  --master-key-file "$task_tmp/master.key" \
  --jwt-private-key-file "$task_tmp/jwt.seed" \
  --relay-internal-url http://127.0.0.1:19003 \
  --relay-service-token-file "$task_tmp/relay.token" \
  --admin-assets-dir "$stage/assets/admin" \
  --portal-assets-dir "$stage/assets/portal" \
  --public-origin http://127.0.0.1:19004 \
  --diagnostics-log-dir "$task_tmp" \
  > "$task_tmp/hub.jsonl" 2> "$task_tmp/hub.stderr.log" &
hub_pid=$!

ready=false
for _ in $(seq 1 120); do
  if curl -fsS http://127.0.0.1:19004/ready >/dev/null 2>&1; then ready=true; break; fi
  sleep 0.25
done
[[ "$ready" == true ]] || { echo "Hub did not become ready" >&2; exit 1; }
curl -fsS http://127.0.0.1:19004/live | grep -q '"live":true'
curl -fsS http://127.0.0.1:19004/ready | grep -q '"ready":true'
curl -fsS http://127.0.0.1:19004/.well-known/measix | grep -q clientApiBase
curl -fsS http://127.0.0.1:19004/admin/ | grep -q 'id=q-app'
curl -fsS http://127.0.0.1:19004/portal/ | grep -q '<div id="app"></div>'
"$qemu_bin" "$stage/bin/control-hub" backup --db "$task_tmp/hub.db" --output "$task_tmp/hub.backup.db" >/dev/null
"$qemu_bin" "$stage/bin/control-hub" check --db "$task_tmp/hub.backup.db" | grep -q 'integrity=ok'
"$qemu_bin" "$stage/bin/runtime-relay" backup --spool "$task_tmp/relay.db" --output "$task_tmp/relay.backup.db" >/dev/null
[[ -s "$task_tmp/relay.backup.db" ]]
grep -q '"service":"hub"' "$task_tmp/hub.jsonl"
grep -q '"service":"relay"' "$task_tmp/relay.jsonl"
echo ARM64_NO_SOURCE_SMOKE_OK
