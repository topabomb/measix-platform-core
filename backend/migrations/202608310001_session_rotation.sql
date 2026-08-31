ALTER TABLE sessions ADD COLUMN previous_refresh_digest blob NULL;
CREATE UNIQUE INDEX sessions_previous_refresh_digest ON sessions(previous_refresh_digest);
ALTER TABLE sessions ADD COLUMN refresh_request_key text NULL;
ALTER TABLE sessions ADD COLUMN refresh_replay_until datetime NULL;
ALTER TABLE sessions ADD COLUMN refresh_response_ciphertext blob NULL;
ALTER TABLE devices ADD COLUMN name text NOT NULL DEFAULT '';

-- Never extend an existing lease. Apply the new idle policy to Android only.
UPDATE sessions SET expires_at = datetime(COALESCE(last_used_at, created_at), '+7 days')
WHERE channel = 'ANDROID'
  AND julianday(expires_at) > julianday(COALESCE(last_used_at, created_at), '+7 days');
