-- Optional workspaceservice. No accounts, credentials or remote side effects on upgrade.
CREATE TABLE workspace_services (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, config_revision INTEGER NOT NULL,
 active_config_revision INTEGER, enabled BOOLEAN NOT NULL, state TEXT NOT NULL,
 mcp_server_id TEXT NOT NULL UNIQUE, runtime_route_id TEXT NOT NULL UNIQUE,
 diagnostic_code TEXT, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX one_enabled_agent_space_integration ON workspace_services(enabled) WHERE enabled=1;
CREATE TABLE workspace_service_configs (
 id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_service_id TEXT NOT NULL REFERENCES workspace_services(id),
 revision INTEGER NOT NULL, config_json BLOB NOT NULL, created_by_user_id TEXT NOT NULL, created_at DATETIME NOT NULL,
 UNIQUE(workspace_service_id,revision)
);
CREATE TABLE agent_spaces (
 id TEXT PRIMARY KEY, workspace_service_id TEXT NOT NULL REFERENCES workspace_services(id), remote_username TEXT NOT NULL,
 agent_space_id TEXT, binding_revision INTEGER NOT NULL, intent TEXT NOT NULL CHECK(intent IN ('CONNECTED','DISCONNECTED','DELETED')),
 state TEXT NOT NULL, mcp_secret_id TEXT, mcp_secret_version INTEGER, dav_secret_id TEXT, dav_secret_version INTEGER,
 dav_confirmed BOOLEAN NOT NULL, remote_active BOOLEAN NOT NULL, stop_pending BOOLEAN NOT NULL,
 applied_control_revision INTEGER, diagnostic_code TEXT, observed_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
 UNIQUE(workspace_service_id,remote_username), UNIQUE(workspace_service_id,agent_space_id)
);
CREATE TABLE workspace_operations (
 id TEXT PRIMARY KEY, workspace_service_id TEXT NOT NULL REFERENCES workspace_services(id), user_id TEXT,
 action TEXT NOT NULL, idempotency_key TEXT NOT NULL, request_hash TEXT NOT NULL,
 config_revision INTEGER NOT NULL, binding_revision INTEGER NOT NULL, target_json BLOB NOT NULL,
 state TEXT NOT NULL, step TEXT NOT NULL, result_json BLOB, candidate_secret_id TEXT, candidate_secret_version INTEGER,
 activation_id TEXT, diagnostic_code TEXT, evidence TEXT, created_by_user_id TEXT NOT NULL,
 created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
 UNIQUE(created_by_user_id,idempotency_key)
);
CREATE INDEX workspace_operations_state_updated_at ON workspace_operations(state,updated_at);
CREATE TABLE workspace_audits (
 id INTEGER PRIMARY KEY AUTOINCREMENT, request_id TEXT NOT NULL, actor_id TEXT NOT NULL, user_id TEXT NOT NULL,
 agent_space_id TEXT NOT NULL, action TEXT NOT NULL, path TEXT NOT NULL, outcome TEXT NOT NULL, bytes INTEGER NOT NULL,
 created_at DATETIME NOT NULL, completed_at DATETIME
);
