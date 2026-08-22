-- MEASIX S0 upgrade migration from previous schema v0.1 to current.
-- This is a test-only migration that demonstrates the upgrade path from
-- a previous supported schema version. It adds any missing tables/columns
-- using IF NOT EXISTS to be safe on top of an existing schema.
-- In production, Atlas would manage this upgrade; this fixture validates
-- the migration replay path preserves data integrity.

-- All tables from the previous schema are preserved; new tables or columns
-- would be added here. Since S0 has only one schema version, this migration
-- is effectively a no-op (the schema is already current from the fixture).
-- The test verifies that data seeded into the previous schema survives
-- a migration replay.

-- Verify data integrity: no destructive operations.
SELECT 1;
