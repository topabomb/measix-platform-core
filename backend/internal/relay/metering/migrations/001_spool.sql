PRAGMA journal_mode=WAL;
PRAGMA synchronous=FULL;

CREATE TABLE IF NOT EXISTS runtime_request_spool(
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id TEXT NOT NULL UNIQUE,
  state TEXT NOT NULL CHECK(state IN ('ADMITTED','STARTED','SETTLEMENT_PENDING')),
  admission_json TEXT NOT NULL,
  started_at TEXT,
  settlement_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT,
  last_error_code TEXT,
  CHECK((state='ADMITTED' AND started_at IS NULL AND settlement_json IS NULL) OR
        (state='STARTED' AND started_at IS NOT NULL AND settlement_json IS NULL) OR
        (state='SETTLEMENT_PENDING' AND started_at IS NOT NULL AND settlement_json IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_runtime_request_spool_due
  ON runtime_request_spool(state,next_attempt_at,seq);
