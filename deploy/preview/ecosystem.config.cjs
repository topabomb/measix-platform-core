'use strict'

const path = require('node:path')
const serviceRoot = __dirname
const at = (...parts) => path.join(serviceRoot, ...parts)

module.exports = {
  apps: [
    {
      name: 'measix-relay',
      script: at('run.sh'), args: ['relay'], interpreter: 'bash',
      exec_mode: 'fork', instances: 1,
      cwd: serviceRoot, uid: 'measix', gid: 'measix',
      autorestart: true, restart_delay: 3000, max_restarts: 10, min_uptime: 10000,
      kill_timeout: 40000, listen_timeout: 15000,
      out_file: at('logs', 'relay.jsonl'), error_file: at('logs', 'relay.stderr.log'),
      time: false, merge_logs: true,
    },
    {
      name: 'measix-hub',
      script: at('run.sh'), args: ['hub'], interpreter: 'bash',
      exec_mode: 'fork', instances: 1,
      cwd: serviceRoot, uid: 'measix', gid: 'measix',
      autorestart: true, restart_delay: 3000, max_restarts: 10, min_uptime: 10000,
      kill_timeout: 40000, listen_timeout: 15000,
      out_file: at('logs', 'hub.jsonl'), error_file: at('logs', 'hub.stderr.log'),
      time: false, merge_logs: true,
    },
  ],
}
