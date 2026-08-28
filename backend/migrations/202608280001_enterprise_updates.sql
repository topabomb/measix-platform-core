-- S0.2 Enterprise Updates table. Independent feed revision, not part of Managed Snapshot.
CREATE TABLE enterprise_updates(id TEXT PRIMARY KEY,title TEXT NOT NULL,content TEXT NOT NULL,status TEXT NOT NULL,published_at DATETIME,feed_revision INTEGER NOT NULL,created_by_user_id TEXT NOT NULL REFERENCES users(id),created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL);
CREATE INDEX idx_enterprise_updates_status ON enterprise_updates(status);
CREATE INDEX idx_enterprise_updates_feed_revision ON enterprise_updates(feed_revision);
CREATE INDEX idx_enterprise_updates_published_at ON enterprise_updates(published_at);
