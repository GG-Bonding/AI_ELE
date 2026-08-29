-- Phase 0 bootstrap schema.
-- Domain tables (episodes, experiences, …) arrive in later phases.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS aee_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO aee_meta (key, value)
VALUES ('schema_bootstrap', 'phase0')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
