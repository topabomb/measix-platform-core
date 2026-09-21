'use strict'

const path = require('node:path')
const root = process.env.MEASIX_ROOT
if (!root || !path.isAbsolute(root) || path.parse(root).root === path.resolve(root)) {
  throw new Error('MEASIX_ROOT must be an explicit absolute deployment directory, not a filesystem root')
}
const at = (...parts) => path.join(root, ...parts)

module.exports = {
  apps: [
    {
      name: 'measix-relay',
      script: at('current', 'bin', 'runtime-relay'),
      interpreter: 'none', exec_mode: 'fork', instances: 1,
      cwd: at('current'), uid: 'measix', gid: 'measix',
      args: [
        '--public-listen', '127.0.0.1:9002', '--internal-listen', '127.0.0.1:9003',
        '--spool', at('data', 'relay', 'relay-spool.db'),
        '--hub-internal-url', 'http://127.0.0.1:9001',
        '--hub-service-token-file', at('secrets', 'relay-service.token'),
      ],
      autorestart: true, restart_delay: 3000, max_restarts: 10, min_uptime: 10000,
      kill_timeout: 40000, listen_timeout: 15000,
      out_file: at('logs', 'relay.jsonl'), error_file: at('logs', 'relay.stderr.log'),
      time: false, merge_logs: true,
    },
    {
      name: 'measix-hub',
      script: at('current', 'bin', 'control-hub'),
      interpreter: 'none', exec_mode: 'fork', instances: 1,
      cwd: at('current'), uid: 'measix', gid: 'measix',
      args: ['run'],
      env: {
        HUB_LISTEN_ADDR: '127.0.0.1:9004', HUB_INTERNAL_LISTEN_ADDR: '127.0.0.1:9001',
        HUB_DB_PATH: at('data', 'hub', 'hub.db'),
        HUB_MASTER_KEY_FILE: at('secrets', 'master.key'),
        HUB_JWT_PRIVATE_KEY_FILE: at('secrets', 'jwt-ed25519.seed'),
        HUB_RELAY_SERVICE_TOKEN_FILE: at('secrets', 'relay-service.token'),
        RELAY_INTERNAL_URL: 'http://127.0.0.1:9003',
        HUB_ADMIN_ASSETS_DIR: at('current', 'assets', 'admin'),
        HUB_PORTAL_ASSETS_DIR: at('current', 'assets', 'portal'),
        HUB_DIAGNOSTICS_LOG_DIR: at('logs'),
        HUB_PUBLIC_ORIGIN: process.env.MEASIX_PUBLIC_ORIGIN || '',
      },
      autorestart: true, restart_delay: 3000, max_restarts: 10, min_uptime: 10000,
      kill_timeout: 40000, listen_timeout: 15000,
      out_file: at('logs', 'hub.jsonl'), error_file: at('logs', 'hub.stderr.log'),
      time: false, merge_logs: true,
    },
  ],
}
