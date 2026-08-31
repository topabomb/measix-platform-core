ALTER TABLE deployments ADD COLUMN timezone text NOT NULL DEFAULT 'UTC';
ALTER TABLE deployments ADD COLUMN feed_revision integer NOT NULL DEFAULT 0;
-- Preserve revision monotonicity for databases that already served Feed items.
UPDATE deployments SET feed_revision = COALESCE((SELECT MAX(feed_revision) FROM enterprise_updates), 0);
