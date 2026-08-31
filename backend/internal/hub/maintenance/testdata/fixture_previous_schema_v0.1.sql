-- MEASIX S0 previous-supported schema (v0.1 baseline).
-- This fixture represents the previous supported schema version.
-- Tests apply all real migrations after 202608190001_initial on top of it.
-- Tables that already exist in this fixture must be preserved by the upgrade.
-- The upgrade should add new tables/columns without dropping existing data.
PRAGMA foreign_keys = ON;
CREATE TABLE deployments(id TEXT PRIMARY KEY,name TEXT NOT NULL,status TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE users(id TEXT PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT,display_name TEXT NOT NULL,role TEXT NOT NULL,status TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE devices(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),installation_id TEXT UNIQUE,status TEXT NOT NULL,app_version TEXT,created_at DATETIME NOT NULL,last_seen_at DATETIME,revoked_at DATETIME);
CREATE INDEX idx_devices_user_status ON devices(user_id,status);
CREATE TABLE enrollments(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),token_digest BLOB NOT NULL UNIQUE,expires_at DATETIME NOT NULL,consumed_at DATETIME,created_by_user_id TEXT NOT NULL REFERENCES users(id),created_at DATETIME NOT NULL);
CREATE TABLE sessions(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),device_id TEXT REFERENCES devices(id),channel TEXT NOT NULL,refresh_digest BLOB UNIQUE,expires_at DATETIME NOT NULL,status TEXT NOT NULL,created_at DATETIME NOT NULL,last_used_at DATETIME,revoked_at DATETIME);
CREATE INDEX idx_sessions_user_status_expiry ON sessions(user_id,status,expires_at);
CREATE TABLE managed_drafts(id TEXT PRIMARY KEY,draft_revision INTEGER NOT NULL,content_json BLOB NOT NULL,updated_by_user_id TEXT NOT NULL REFERENCES users(id),updated_at DATETIME NOT NULL);
CREATE TABLE managed_releases(id TEXT PRIMARY KEY,managed_generation INTEGER NOT NULL UNIQUE,status TEXT NOT NULL,release_content_json BLOB NOT NULL,snapshot_schema_version INTEGER NOT NULL,snapshot_json BLOB NOT NULL,snapshot_hash TEXT NOT NULL,source_draft_revision INTEGER NOT NULL,created_by_user_id TEXT NOT NULL REFERENCES users(id),created_at DATETIME NOT NULL);
CREATE INDEX idx_release_status ON managed_releases(status);
CREATE TABLE managed_states(id TEXT PRIMARY KEY,active_release_id TEXT REFERENCES managed_releases(id),active_managed_generation INTEGER NOT NULL DEFAULT 0,desired_control_revision INTEGER NOT NULL DEFAULT 0,desired_bundle_hash TEXT,managed_state_revision INTEGER NOT NULL DEFAULT 0,runtime_status TEXT NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE upstreams(id TEXT PRIMARY KEY,name TEXT NOT NULL,config_revision INTEGER NOT NULL,active_config_revision INTEGER,status TEXT NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE upstream_config_revisions(id INTEGER PRIMARY KEY AUTOINCREMENT,upstream_id TEXT NOT NULL REFERENCES upstreams(id),revision INTEGER NOT NULL,config_json BLOB NOT NULL,created_by_user_id TEXT NOT NULL REFERENCES users(id),created_at DATETIME NOT NULL,UNIQUE(upstream_id,revision));
CREATE TABLE secrets(id TEXT PRIMARY KEY,name TEXT NOT NULL,latest_secret_version INTEGER NOT NULL,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE TABLE secret_versions(id INTEGER PRIMARY KEY AUTOINCREMENT,secret_id TEXT NOT NULL REFERENCES secrets(id),secret_version INTEGER NOT NULL,encrypted_payload BLOB NOT NULL,key_version INTEGER NOT NULL,created_by_user_id TEXT NOT NULL REFERENCES users(id),created_at DATETIME NOT NULL,UNIQUE(secret_id,secret_version));
CREATE TABLE activations(id TEXT PRIMARY KEY,kind TEXT NOT NULL,state TEXT NOT NULL,idempotency_key TEXT NOT NULL,request_hash TEXT NOT NULL,control_revision INTEGER NOT NULL,bundle_hash TEXT NOT NULL,target_generation INTEGER,target_descriptor_json BLOB NOT NULL,subject_id TEXT,pending_operation_json BLOB,error_code TEXT,created_by_user_id TEXT NOT NULL REFERENCES users(id),created_at DATETIME NOT NULL,completed_at DATETIME);
CREATE INDEX idx_activations_state_created ON activations(state,created_at);
CREATE TABLE idempotency_records(id INTEGER PRIMARY KEY AUTOINCREMENT,admin_user_id TEXT NOT NULL REFERENCES users(id),method TEXT NOT NULL,normalized_path TEXT NOT NULL,idempotency_key TEXT NOT NULL,request_hash TEXT NOT NULL,activation_id TEXT REFERENCES activations(id),status_code INTEGER,response_json BLOB,created_at DATETIME NOT NULL,UNIQUE(admin_user_id,method,normalized_path,idempotency_key));
CREATE TABLE request_usages(id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL UNIQUE,interaction_id TEXT,deployment_id TEXT NOT NULL,user_id NOT NULL REFERENCES users(id),device_id TEXT REFERENCES devices(id),resource_id TEXT NOT NULL,runtime_route_id TEXT NOT NULL,upstream_id TEXT NOT NULL REFERENCES upstreams(id),managed_generation INTEGER NOT NULL,control_revision INTEGER NOT NULL,started_at DATETIME NOT NULL,completed_at DATETIME NOT NULL,forwarded INTEGER NOT NULL,http_status INTEGER NOT NULL,upstream_http_status INTEGER,request_bytes INTEGER NOT NULL,response_bytes INTEGER NOT NULL,duration_ms INTEGER NOT NULL,error_class TEXT,ingested_at DATETIME NOT NULL);
CREATE INDEX idx_usage_completed_user ON request_usages(completed_at,user_id);
CREATE INDEX idx_usage_completed_resource ON request_usages(completed_at,resource_id);
CREATE TABLE semantic_usages(id TEXT PRIMARY KEY,request_id TEXT,upstream_id TEXT NOT NULL REFERENCES upstreams(id),resource_id TEXT,source_event_id TEXT,meter TEXT NOT NULL,quantity_decimal TEXT NOT NULL,completeness TEXT NOT NULL,provider_cost TEXT,currency TEXT,source TEXT NOT NULL,occurred_at DATETIME NOT NULL);
CREATE UNIQUE INDEX idx_semantic_source_dedupe ON semantic_usages(upstream_id,source_event_id) WHERE source_event_id IS NOT NULL;
CREATE INDEX idx_semantic_request ON semantic_usages(request_id);
CREATE TABLE pricing_rules(id TEXT PRIMARY KEY,resource_id TEXT,upstream_id TEXT REFERENCES upstreams(id),meter TEXT NOT NULL,unit_size TEXT NOT NULL,unit_price_decimal TEXT NOT NULL,currency TEXT NOT NULL,effective_from DATETIME NOT NULL,effective_to DATETIME);
CREATE INDEX idx_pricing_scope ON pricing_rules(resource_id,upstream_id,meter,effective_from);
