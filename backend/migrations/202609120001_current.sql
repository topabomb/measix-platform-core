-- Current unpublished MEASIX Control Hub schema. Initialize empty databases only.
PRAGMA foreign_keys = ON;
CREATE TABLE activations(id TEXT PRIMARY KEY,kind TEXT NOT NULL,state TEXT NOT NULL,idempotency_key TEXT NOT NULL,request_hash TEXT NOT NULL,control_revision INTEGER NOT NULL,bundle_hash TEXT NOT NULL,target_generation INTEGER,target_descriptor_json BLOB NOT NULL,subject_id TEXT,pending_operation_json BLOB,error_code TEXT,created_by_user_id TEXT NOT NULL,created_at DATETIME NOT NULL,completed_at DATETIME);
CREATE TABLE deployments(id TEXT PRIMARY KEY,name TEXT NOT NULL,status TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL, timezone text NOT NULL DEFAULT 'UTC', public_origin text NOT NULL DEFAULT '', feed_revision integer NOT NULL DEFAULT 0);
CREATE TABLE deployment_setting_audits(id INTEGER PRIMARY KEY AUTOINCREMENT,deployment_id TEXT NOT NULL,actor_user_id TEXT NOT NULL,old_name TEXT NOT NULL,new_name TEXT NOT NULL,old_public_origin TEXT NOT NULL DEFAULT '',new_public_origin TEXT NOT NULL DEFAULT '',created_at DATETIME NOT NULL);
CREATE INDEX deploymentsettingaudit_deployment_id_created_at ON deployment_setting_audits(deployment_id,created_at);
CREATE TABLE devices(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),installation_id TEXT UNIQUE,status TEXT NOT NULL,app_version TEXT,created_at DATETIME NOT NULL,last_seen_at DATETIME,revoked_at DATETIME, name text NOT NULL DEFAULT '');
CREATE TABLE enrollments(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),token_digest BLOB NOT NULL UNIQUE,expires_at DATETIME NOT NULL,consumed_at DATETIME,created_by_user_id TEXT NOT NULL,created_at DATETIME NOT NULL);
CREATE TABLE enterprise_updates(id TEXT PRIMARY KEY,title TEXT NOT NULL,content TEXT NOT NULL,content_format TEXT NOT NULL DEFAULT 'PLAIN',category TEXT NOT NULL DEFAULT 'NOTICE',severity TEXT NOT NULL DEFAULT 'INFO',status TEXT NOT NULL,published_at DATETIME,feed_revision INTEGER NOT NULL,created_by_user_id TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE idempotency_records(id INTEGER PRIMARY KEY AUTOINCREMENT,admin_user_id TEXT NOT NULL REFERENCES users(id),method TEXT NOT NULL,normalized_path TEXT NOT NULL,idempotency_key TEXT NOT NULL,request_hash TEXT NOT NULL,activation_id TEXT REFERENCES activations(id),status_code INTEGER,response_json BLOB,created_at DATETIME NOT NULL,UNIQUE(admin_user_id,method,normalized_path,idempotency_key));
CREATE TABLE managed_drafts(id TEXT PRIMARY KEY,draft_revision INTEGER NOT NULL,content_json BLOB NOT NULL,updated_by_user_id TEXT NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE managed_releases(id TEXT PRIMARY KEY,managed_generation INTEGER NOT NULL UNIQUE,status TEXT NOT NULL,release_content_json BLOB NOT NULL,snapshot_json BLOB NOT NULL,snapshot_hash TEXT NOT NULL,source_draft_revision INTEGER NOT NULL,created_by_user_id TEXT NOT NULL,created_at DATETIME NOT NULL);
CREATE TABLE managed_states(id TEXT PRIMARY KEY,active_release_id TEXT REFERENCES managed_releases(id),active_managed_generation INTEGER NOT NULL DEFAULT 0,desired_control_revision INTEGER NOT NULL DEFAULT 0,desired_bundle_hash TEXT,managed_state_revision INTEGER NOT NULL DEFAULT 0,runtime_status TEXT NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE `portal_sessions` (
 `id` text NOT NULL,
 `session_id` text NOT NULL,
 `origin` text NOT NULL,
 `ticket_digest` blob NOT NULL,
 `cookie_digest` blob NULL,
 `grant_expires_at` datetime NOT NULL,
 `expires_at` datetime NOT NULL,
 `consumed` bool NOT NULL DEFAULT false,
 `revoked` bool NOT NULL DEFAULT false,
 PRIMARY KEY (`id`)
);
CREATE TABLE pricing_rules(id TEXT PRIMARY KEY,resource_id TEXT,upstream_id TEXT REFERENCES upstreams(id),meter TEXT NOT NULL,unit_size TEXT NOT NULL,unit_price_decimal TEXT NOT NULL,currency TEXT NOT NULL,effective_from DATETIME NOT NULL,effective_to DATETIME);
CREATE TABLE "request_usages"(id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL UNIQUE,interaction_id TEXT,deployment_id TEXT NOT NULL,user_id TEXT NOT NULL REFERENCES users(id),device_id TEXT REFERENCES devices(id),resource_id TEXT NOT NULL,resource_kind TEXT NOT NULL,client_protocol TEXT NOT NULL,runtime_route_id TEXT NOT NULL,upstream_id TEXT NOT NULL REFERENCES upstreams(id),managed_generation INTEGER NOT NULL,control_revision INTEGER NOT NULL,started_at DATETIME NOT NULL,completed_at DATETIME NOT NULL,forwarded INTEGER NOT NULL,http_status INTEGER NOT NULL,upstream_http_status INTEGER,request_bytes INTEGER NOT NULL,response_bytes INTEGER NOT NULL,duration_ms INTEGER NOT NULL,error_class TEXT,request_completeness TEXT NOT NULL,settlement_state TEXT NOT NULL,settlement_revision INTEGER NOT NULL,budget_revision INTEGER NOT NULL,ingested_at DATETIME NOT NULL);
CREATE TABLE secret_versions(id INTEGER PRIMARY KEY AUTOINCREMENT,secret_id TEXT NOT NULL REFERENCES secrets(id),secret_version INTEGER NOT NULL,encrypted_payload BLOB NOT NULL,key_version INTEGER NOT NULL,created_by_user_id TEXT NOT NULL,created_at DATETIME NOT NULL,UNIQUE(secret_id,secret_version));
CREATE TABLE secrets(id TEXT PRIMARY KEY,name TEXT NOT NULL,latest_secret_version INTEGER NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE semantic_usages(id TEXT PRIMARY KEY,request_id TEXT NOT NULL,settlement_revision INTEGER NOT NULL,source_event_id TEXT NOT NULL,meter TEXT NOT NULL,quantity_units INTEGER NOT NULL,quantity_decimal TEXT NOT NULL,completeness TEXT NOT NULL,provider_cost TEXT,currency TEXT,source TEXT NOT NULL,occurred_at DATETIME NOT NULL,UNIQUE(request_id,settlement_revision,meter));
CREATE TABLE usage_details(id TEXT PRIMARY KEY,request_id TEXT NOT NULL,settlement_revision INTEGER NOT NULL,name TEXT NOT NULL,quantity_units INTEGER NOT NULL,source TEXT NOT NULL,completeness TEXT NOT NULL,occurred_at DATETIME NOT NULL,UNIQUE(request_id,settlement_revision,name));
CREATE TABLE usage_events(id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL,revision INTEGER NOT NULL CHECK(revision > 0),event_hash TEXT NOT NULL,source_event_id TEXT NOT NULL UNIQUE,payload_json BLOB NOT NULL,created_at DATETIME NOT NULL,UNIQUE(request_id,revision));
CREATE TABLE sessions(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),device_id TEXT REFERENCES devices(id),channel TEXT NOT NULL,refresh_digest BLOB UNIQUE,expires_at DATETIME NOT NULL,status TEXT NOT NULL,created_at DATETIME NOT NULL,last_used_at DATETIME,revoked_at DATETIME, previous_refresh_digest blob NULL, refresh_request_key text NULL, refresh_replay_until datetime NULL, refresh_response_ciphertext blob NULL, applied_managed_generation integer NULL, applied_snapshot_hash text NULL, applied_reported_at datetime NULL);
CREATE TABLE upstream_config_revisions(id INTEGER PRIMARY KEY AUTOINCREMENT,upstream_id TEXT NOT NULL REFERENCES upstreams(id),revision INTEGER NOT NULL,config_json BLOB NOT NULL,created_by_user_id TEXT NOT NULL,created_at DATETIME NOT NULL,UNIQUE(upstream_id,revision));
CREATE TABLE upstreams(id TEXT PRIMARY KEY,name TEXT NOT NULL,config_revision INTEGER NOT NULL,active_config_revision INTEGER,status TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE users(id TEXT PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT,display_name TEXT NOT NULL,role TEXT NOT NULL,status TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE deleted_principals(id TEXT PRIMARY KEY,deleted_at DATETIME NOT NULL);
CREATE TABLE deleted_credentials(id INTEGER PRIMARY KEY AUTOINCREMENT,digest BLOB NOT NULL UNIQUE,deleted_at DATETIME NOT NULL);
CREATE TABLE user_budgets(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id TEXT NOT NULL REFERENCES users(id),
 capability TEXT NOT NULL CHECK(capability IN ('MODEL','IMAGE_GENERATION','TTS','ASR','MCP')),
 mode TEXT NOT NULL CHECK(mode IN ('UNLIMITED','LIMITED')),
 source TEXT NOT NULL CHECK(source IN ('DEFAULT','TEMPLATE','EXPLICIT')),
 revision INTEGER NOT NULL CHECK(revision > 0),
 activated_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 updated_by_user_id TEXT NOT NULL,
 UNIQUE(user_id,capability)
);
CREATE TABLE budget_limits(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_budget_id INTEGER NOT NULL REFERENCES user_budgets(id),
 scope_key TEXT NOT NULL,
 period TEXT NOT NULL CHECK(period IN ('DAY','WEEK','MONTH','LIFETIME')),
 meter TEXT NOT NULL CHECK(meter IN ('REQUESTS','REQUESTED_IMAGES','INPUT_TOKENS','OUTPUT_TOKENS','CACHED_TOKENS','TOTAL_TOKENS','CHARACTERS','AUDIO_MILLISECONDS')),
 limit_quantity INTEGER NOT NULL CHECK(limit_quantity >= 0),
 scope_started_at DATETIME NOT NULL,
 effective_from DATETIME NOT NULL,
 effective_to DATETIME,
 created_at DATETIME NOT NULL,
 created_by_user_id TEXT NOT NULL,
 CHECK(effective_to IS NULL OR effective_to > effective_from),
 UNIQUE(user_budget_id,scope_key,meter,effective_from)
);
CREATE TABLE budget_buckets(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 scope_key TEXT NOT NULL,
 period TEXT NOT NULL CHECK(period IN ('DAY','WEEK','MONTH','LIFETIME')),
 period_start DATETIME NOT NULL,
 period_end DATETIME,
 meter TEXT NOT NULL CHECK(meter IN ('REQUESTS','REQUESTED_IMAGES','INPUT_TOKENS','OUTPUT_TOKENS','CACHED_TOKENS','TOTAL_TOKENS','CHARACTERS','AUDIO_MILLISECONDS')),
 settled_quantity INTEGER NOT NULL DEFAULT 0 CHECK(settled_quantity >= 0),
 reserved_quantity INTEGER NOT NULL DEFAULT 0 CHECK(reserved_quantity >= 0),
 updated_at DATETIME NOT NULL,
 CHECK(period_end IS NULL OR period_end > period_start),
 CHECK((period = 'LIFETIME' AND period_end IS NULL) OR (period <> 'LIFETIME' AND period_end IS NOT NULL)),
 UNIQUE(scope_key,period_start,meter)
);
CREATE TABLE budget_requests(
 id TEXT PRIMARY KEY,
 request_hash TEXT NOT NULL,
 deployment_id TEXT NOT NULL REFERENCES deployments(id),
 user_id TEXT NOT NULL REFERENCES users(id),
 interaction_id TEXT,
 device_id TEXT REFERENCES devices(id),
 capability TEXT NOT NULL CHECK(capability IN ('MODEL','IMAGE_GENERATION','TTS','ASR','MCP')),
 resource_id TEXT NOT NULL,
 client_protocol TEXT NOT NULL,
 upstream_id TEXT NOT NULL REFERENCES upstreams(id),
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
 CHECK(started_at IS NULL OR started_at >= admitted_at),
 CHECK(completed_at IS NULL OR completed_at >= admitted_at)
);
CREATE TABLE budget_allocations(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 request_id TEXT NOT NULL REFERENCES budget_requests(id),
 budget_limit_id INTEGER NOT NULL REFERENCES budget_limits(id),
 budget_bucket_id INTEGER NOT NULL REFERENCES budget_buckets(id),
 scope_key TEXT NOT NULL,
 period TEXT NOT NULL CHECK(period IN ('DAY','WEEK','MONTH','LIFETIME')),
 meter TEXT NOT NULL CHECK(meter IN ('REQUESTS','REQUESTED_IMAGES','INPUT_TOKENS','OUTPUT_TOKENS','CACHED_TOKENS','TOTAL_TOKENS','CHARACTERS','AUDIO_MILLISECONDS')),
 reserved_quantity INTEGER NOT NULL DEFAULT 0 CHECK(reserved_quantity >= 0),
 reservation_released INTEGER NOT NULL DEFAULT 0 CHECK(reservation_released IN (0,1)),
 settled_quantity INTEGER NOT NULL DEFAULT 0 CHECK(settled_quantity >= 0),
 resolved INTEGER NOT NULL DEFAULT 0 CHECK(resolved IN (0,1)),
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 UNIQUE(request_id,scope_key,meter)
);
CREATE TABLE budget_settlements(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 request_id TEXT NOT NULL REFERENCES budget_requests(id),
 revision INTEGER NOT NULL CHECK(revision > 0),
 payload_hash TEXT NOT NULL,
 meters_json BLOB NOT NULL,
 complete INTEGER NOT NULL CHECK(complete IN (0,1)),
 source TEXT NOT NULL CHECK(source IN ('RELAY','ADMIN')),
 outcome TEXT NOT NULL CHECK(outcome IN ('SETTLED','CORRECTED','RECONCILIATION')),
 reported_by TEXT NOT NULL,
 created_at DATETIME NOT NULL,
 UNIQUE(request_id,revision)
);
CREATE TABLE budget_reconciliations(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 request_id TEXT NOT NULL UNIQUE REFERENCES budget_requests(id),
 state TEXT NOT NULL CHECK(state IN ('OPEN','RESOLVED')),
 reason TEXT NOT NULL,
 opened_at DATETIME NOT NULL,
 resolved_at DATETIME,
 resolved_by_actor TEXT,
 resolution_action TEXT CHECK(resolution_action IN ('RELIABLE_SETTLEMENT','CONFIRM_USAGE','RELEASE_UNCERTAIN')),
 resolution_reason TEXT,
 CHECK((state = 'OPEN' AND resolved_at IS NULL AND resolved_by_actor IS NULL AND resolution_action IS NULL AND resolution_reason IS NULL) OR (state = 'RESOLVED' AND resolved_at IS NOT NULL AND resolved_by_actor IS NOT NULL AND resolution_action IS NOT NULL AND resolution_reason IS NOT NULL))
);
CREATE TABLE budget_audits(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id TEXT NOT NULL REFERENCES users(id),
 capability TEXT NOT NULL CHECK(capability IN ('MODEL','IMAGE_GENERATION','TTS','ASR','MCP')),
 user_budget_id INTEGER REFERENCES user_budgets(id),
 budget_revision INTEGER NOT NULL DEFAULT 0 CHECK(budget_revision >= 0),
 request_id TEXT REFERENCES budget_requests(id),
 actor_user_id TEXT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('CREATE','UPDATE','APPLY_TEMPLATE','CLEAR_OVERRIDE','UNASSIGN_TEMPLATE','RESOLVE_RECONCILIATION')),
 reason TEXT NOT NULL,
 before_json BLOB,
 after_json BLOB NOT NULL,
 created_at DATETIME NOT NULL
);
CREATE TABLE budget_templates(
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 description TEXT NOT NULL,
 rules_json BLOB NOT NULL,
 revision INTEGER NOT NULL CHECK(revision > 0),
 created_at DATETIME NOT NULL,
 created_by_user_id TEXT NOT NULL,
 updated_at DATETIME NOT NULL,
 updated_by_user_id TEXT NOT NULL
);
CREATE TABLE budget_template_assignments(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id TEXT NOT NULL REFERENCES users(id),
 budget_template_id TEXT NOT NULL REFERENCES budget_templates(id),
 revision INTEGER NOT NULL CHECK(revision > 0),
 assigned_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 updated_by_user_id TEXT NOT NULL,
 UNIQUE(user_id)
);
CREATE TABLE budget_template_audits(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 budget_template_id TEXT,
 user_id TEXT,
 template_revision INTEGER NOT NULL DEFAULT 0 CHECK(template_revision >= 0),
 assignment_revision INTEGER NOT NULL DEFAULT 0 CHECK(assignment_revision >= 0),
 actor_user_id TEXT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('CREATE','UPDATE','DELETE','ASSIGN','REASSIGN','UNASSIGN')),
 reason TEXT NOT NULL,
 before_json BLOB,
 after_json BLOB,
 created_at DATETIME NOT NULL
);
CREATE INDEX idx_activations_state_created ON activations(state,created_at);
CREATE INDEX idx_devices_user_status ON devices(user_id,status);
CREATE INDEX idx_enterprise_updates_category ON enterprise_updates(category);
CREATE INDEX idx_enterprise_updates_feed_revision ON enterprise_updates(feed_revision);
CREATE INDEX idx_enterprise_updates_published_at ON enterprise_updates(published_at);
CREATE INDEX idx_enterprise_updates_severity ON enterprise_updates(severity);
CREATE INDEX idx_enterprise_updates_status ON enterprise_updates(status);
CREATE INDEX idx_pricing_scope ON pricing_rules(resource_id,upstream_id,meter,effective_from);
CREATE INDEX idx_release_status ON managed_releases(status);
CREATE INDEX idx_semantic_request ON semantic_usages(request_id,meter,settlement_revision);
CREATE INDEX idx_usage_details_request ON usage_details(request_id,settlement_revision);
CREATE INDEX idx_sessions_user_status_expiry ON sessions(user_id,status,expires_at);
CREATE INDEX idx_usage_completed_resource ON request_usages(completed_at,resource_id);
CREATE INDEX idx_usage_completed_user ON request_usages(completed_at,user_id);
CREATE INDEX idx_usage_completed_protocol ON request_usages(completed_at,client_protocol);
CREATE INDEX idx_usage_completed_kind ON request_usages(completed_at,resource_kind);
CREATE UNIQUE INDEX `portal_sessions_cookie_digest_key` ON `portal_sessions` (`cookie_digest`);
CREATE UNIQUE INDEX `portal_sessions_ticket_digest_key` ON `portal_sessions` (`ticket_digest`);
CREATE UNIQUE INDEX sessions_previous_refresh_digest ON sessions(previous_refresh_digest);
CREATE UNIQUE INDEX idx_budget_limits_active_meter ON budget_limits(user_budget_id,period,meter) WHERE effective_to IS NULL;
CREATE INDEX idx_budget_limits_scope ON budget_limits(scope_key,meter,effective_from);
CREATE INDEX idx_budget_limits_effective ON budget_limits(user_budget_id,effective_from,effective_to);
CREATE INDEX idx_budget_buckets_period_end ON budget_buckets(period_end);
CREATE INDEX idx_budget_allocations_bucket ON budget_allocations(budget_bucket_id);
CREATE INDEX idx_budget_requests_inflight ON budget_requests(user_id,capability,state);
CREATE INDEX idx_budget_requests_state_updated ON budget_requests(state,updated_at);
CREATE INDEX idx_budget_templates_name ON budget_templates(name,id);
CREATE INDEX idx_budget_template_assignments_template ON budget_template_assignments(budget_template_id,user_id);
CREATE INDEX idx_budget_template_audits_template ON budget_template_audits(budget_template_id,created_at);
CREATE INDEX idx_budget_template_audits_user ON budget_template_audits(user_id,created_at);
CREATE INDEX idx_budget_settlements_history ON budget_settlements(request_id,created_at);
CREATE INDEX idx_budget_reconciliations_state_opened ON budget_reconciliations(state,opened_at);
CREATE INDEX idx_budget_audits_subject ON budget_audits(user_id,capability,created_at);
CREATE INDEX idx_budget_audits_request ON budget_audits(request_id,created_at);
