ALTER TABLE managed_states ADD COLUMN last_assigned_generation INTEGER NOT NULL DEFAULT 0;
UPDATE managed_states SET last_assigned_generation = MAX(
  active_managed_generation,
  COALESCE((SELECT MAX(managed_generation) FROM managed_releases), 0),
  COALESCE((SELECT MAX(target_generation) FROM activations), 0)
);
ALTER TABLE managed_releases ADD COLUMN diff_summary_json BLOB;
ALTER TABLE deployments ADD COLUMN release_retention_json BLOB;
ALTER TABLE deployments ADD COLUMN release_retention_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE deployments ADD COLUMN release_cleanup_at DATETIME;
ALTER TABLE budget_requests ADD COLUMN resource_display_name TEXT NOT NULL DEFAULT '';

-- Capture original resource names in admission facts, including requests whose
-- settlement is still delayed. No current draft is used as historical context.
WITH names AS (
  SELECT managed_generation, json_extract(r.value, '$.providerId') AS resource_id, json_extract(r.value, '$.displayName') AS name
  FROM managed_releases, json_each(CAST(snapshot_json AS TEXT), '$.providers') r
  UNION ALL
  SELECT managed_generation, json_extract(r.value, '$.modelId'), json_extract(r.value, '$.displayName')
  FROM managed_releases, json_each(CAST(snapshot_json AS TEXT), '$.models') r
  UNION ALL
  SELECT managed_generation, json_extract(r.value, '$.imageId'), json_extract(r.value, '$.displayName')
  FROM managed_releases, json_each(CAST(snapshot_json AS TEXT), '$.imageGenerators') r
  UNION ALL
  SELECT managed_generation, json_extract(r.value, '$.ttsId'), json_extract(r.value, '$.displayName')
  FROM managed_releases, json_each(CAST(snapshot_json AS TEXT), '$.tts') r
  UNION ALL
  SELECT managed_generation, json_extract(r.value, '$.asrId'), json_extract(r.value, '$.displayName')
  FROM managed_releases, json_each(CAST(snapshot_json AS TEXT), '$.asr') r
  UNION ALL
  SELECT managed_generation, json_extract(r.value, '$.mcpServerId'), json_extract(r.value, '$.displayName')
  FROM managed_releases, json_each(CAST(snapshot_json AS TEXT), '$.mcp') r
)
UPDATE budget_requests SET resource_display_name = COALESCE((
  SELECT name FROM names WHERE names.managed_generation = budget_requests.managed_generation
  AND names.resource_id = budget_requests.resource_id LIMIT 1
), '');

CREATE TABLE release_history_audits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_user_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  details_json BLOB NOT NULL,
  created_at DATETIME NOT NULL
);
