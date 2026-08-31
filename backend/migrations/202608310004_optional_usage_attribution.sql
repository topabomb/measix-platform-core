-- Keep resolved attribution; denied requests may have no resource/route/upstream.
CREATE TABLE request_usages_next(id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL UNIQUE,interaction_id TEXT,deployment_id TEXT NOT NULL,user_id TEXT NOT NULL REFERENCES users(id),device_id TEXT REFERENCES devices(id),resource_id TEXT,runtime_route_id TEXT,upstream_id TEXT REFERENCES upstreams(id),managed_generation INTEGER NOT NULL,control_revision INTEGER NOT NULL,started_at DATETIME NOT NULL,completed_at DATETIME NOT NULL,forwarded INTEGER NOT NULL,http_status INTEGER NOT NULL,upstream_http_status INTEGER,request_bytes INTEGER NOT NULL,response_bytes INTEGER NOT NULL,duration_ms INTEGER NOT NULL,error_class TEXT,ingested_at DATETIME NOT NULL);
INSERT INTO request_usages_next SELECT * FROM request_usages;
DROP TABLE request_usages;
ALTER TABLE request_usages_next RENAME TO request_usages;
CREATE INDEX idx_usage_completed_user ON request_usages(completed_at,user_id);
CREATE INDEX idx_usage_completed_resource ON request_usages(completed_at,resource_id);
