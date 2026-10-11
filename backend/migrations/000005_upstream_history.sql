-- Historical attribution keeps the original upstream ID independently of
-- the operational connection. All other constraints and original bytes survive.
PRAGMA defer_foreign_keys=ON;
CREATE TEMP TABLE upstream_history_sequences(name TEXT PRIMARY KEY,seq INTEGER NOT NULL);
INSERT INTO upstream_history_sequences SELECT name,seq FROM sqlite_sequence WHERE name='request_usages';

CREATE TABLE budget_requests_history(workspace_target_json BLOB,
 id TEXT PRIMARY KEY,
 request_hash TEXT NOT NULL,
 deployment_id TEXT NOT NULL REFERENCES deployments(id),
 user_id TEXT NOT NULL REFERENCES users(id),
 interaction_id TEXT,
 device_id TEXT REFERENCES devices(id),
 capability TEXT NOT NULL CHECK(capability IN ('MODEL','IMAGE_GENERATION','TTS','ASR','MCP')),
 resource_id TEXT NOT NULL,
 client_protocol TEXT NOT NULL,
 upstream_id TEXT,
 managed_generation INTEGER NOT NULL CHECK(managed_generation >= 0),
 control_revision INTEGER NOT NULL CHECK(control_revision >= 0),
 user_budget_id INTEGER REFERENCES user_budgets(id),
 budget_revision INTEGER NOT NULL DEFAULT 0 CHECK(budget_revision >= 0),
 mode TEXT NOT NULL CHECK(mode IN ('UNLIMITED','LIMITED')),
 source TEXT NOT NULL CHECK(source IN ('DEFAULT','TEMPLATE','EXPLICIT')),
 decision_json BLOB NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('DENIED','ADMITTED','STARTED','RECONCILIATION','SETTLED','RELEASED','RESOLVED')),
 admitted_at DATETIME NOT NULL,
 started_at DATETIME,
 completed_at DATETIME,
 last_settlement_revision INTEGER NOT NULL DEFAULT 0 CHECK(last_settlement_revision >= 0),
 last_lifecycle_revision INTEGER NOT NULL DEFAULT 0 CHECK(last_lifecycle_revision >= 0),
 last_lifecycle_hash TEXT,
 terminal_reason TEXT,
 updated_at DATETIME NOT NULL,
 resource_display_name TEXT NOT NULL DEFAULT '',
 CHECK(started_at IS NULL OR started_at >= admitted_at),
 CHECK(completed_at IS NULL OR completed_at >= admitted_at)
, CHECK ((upstream_id IS NOT NULL AND workspace_target_json IS NULL) OR (upstream_id IS NULL AND workspace_target_json IS NOT NULL)));

INSERT INTO budget_requests_history(id,request_hash,deployment_id,user_id,interaction_id,device_id,capability,resource_id,client_protocol,upstream_id,managed_generation,control_revision,user_budget_id,budget_revision,mode,source,decision_json,state,admitted_at,started_at,completed_at,last_settlement_revision,last_lifecycle_revision,last_lifecycle_hash,terminal_reason,updated_at,workspace_target_json,resource_display_name) SELECT id,request_hash,deployment_id,user_id,interaction_id,device_id,capability,resource_id,client_protocol,upstream_id,managed_generation,control_revision,user_budget_id,budget_revision,mode,source,decision_json,state,admitted_at,started_at,completed_at,last_settlement_revision,last_lifecycle_revision,last_lifecycle_hash,terminal_reason,updated_at,workspace_target_json,resource_display_name FROM budget_requests;

DROP TABLE budget_requests;

ALTER TABLE budget_requests_history RENAME TO budget_requests;

CREATE INDEX idx_budget_requests_inflight ON budget_requests(user_id,capability,state);

CREATE INDEX idx_budget_requests_state_updated ON budget_requests(state,updated_at);

CREATE TABLE request_usages_history(workspace_target_json BLOB, id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL UNIQUE,interaction_id TEXT,deployment_id TEXT NOT NULL,user_id TEXT NOT NULL REFERENCES users(id),device_id TEXT REFERENCES devices(id),resource_id TEXT NOT NULL,resource_kind TEXT NOT NULL,client_protocol TEXT NOT NULL,runtime_route_id TEXT NOT NULL,upstream_id TEXT,managed_generation INTEGER NOT NULL,control_revision INTEGER NOT NULL,started_at DATETIME NOT NULL,completed_at DATETIME NOT NULL,forwarded INTEGER NOT NULL,http_status INTEGER NOT NULL,upstream_http_status INTEGER,request_bytes INTEGER NOT NULL,response_bytes INTEGER NOT NULL,duration_ms INTEGER NOT NULL,error_class TEXT,request_completeness TEXT NOT NULL,settlement_state TEXT NOT NULL,settlement_revision INTEGER NOT NULL,budget_revision INTEGER NOT NULL,ingested_at DATETIME NOT NULL, CHECK ((upstream_id IS NOT NULL AND workspace_target_json IS NULL) OR (upstream_id IS NULL AND workspace_target_json IS NOT NULL)));

INSERT INTO request_usages_history(id,request_id,interaction_id,deployment_id,user_id,device_id,resource_id,resource_kind,client_protocol,runtime_route_id,upstream_id,managed_generation,control_revision,started_at,completed_at,forwarded,http_status,upstream_http_status,request_bytes,response_bytes,duration_ms,error_class,request_completeness,settlement_state,settlement_revision,budget_revision,ingested_at,workspace_target_json) SELECT id,request_id,interaction_id,deployment_id,user_id,device_id,resource_id,resource_kind,client_protocol,runtime_route_id,upstream_id,managed_generation,control_revision,started_at,completed_at,forwarded,http_status,upstream_http_status,request_bytes,response_bytes,duration_ms,error_class,request_completeness,settlement_state,settlement_revision,budget_revision,ingested_at,workspace_target_json FROM request_usages;

DROP TABLE request_usages;

ALTER TABLE request_usages_history RENAME TO request_usages;

CREATE INDEX idx_usage_completed_resource ON request_usages(completed_at,resource_id);

CREATE INDEX idx_usage_completed_user ON request_usages(completed_at,user_id);

CREATE INDEX idx_usage_completed_protocol ON request_usages(completed_at,client_protocol);

CREATE INDEX idx_usage_completed_kind ON request_usages(completed_at,resource_kind);

INSERT INTO sqlite_sequence(name,seq) SELECT name,seq FROM upstream_history_sequences old WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence current WHERE current.name=old.name);
UPDATE sqlite_sequence SET seq=MAX(seq,COALESCE((SELECT seq FROM upstream_history_sequences WHERE name='request_usages'),0)) WHERE name='request_usages';

DROP TABLE upstream_history_sequences;

CREATE TABLE pricing_rules_history(id TEXT PRIMARY KEY,resource_id TEXT,upstream_id TEXT,meter TEXT NOT NULL,unit_size TEXT NOT NULL,unit_price_decimal TEXT NOT NULL,currency TEXT NOT NULL,effective_from DATETIME NOT NULL,effective_to DATETIME);

INSERT INTO pricing_rules_history SELECT * FROM pricing_rules;

DROP TABLE pricing_rules;

ALTER TABLE pricing_rules_history RENAME TO pricing_rules;

CREATE INDEX idx_pricing_scope ON pricing_rules(resource_id,upstream_id,meter,effective_from);

-- Validate the complete restored graph before clearing deferred DROP counters.
CREATE TEMP TABLE upstream_history_fk_guard (violations INTEGER CHECK (violations = 0));
INSERT INTO upstream_history_fk_guard SELECT COUNT(*) FROM pragma_foreign_key_check;
DROP TABLE upstream_history_fk_guard;
PRAGMA defer_foreign_keys=OFF;
