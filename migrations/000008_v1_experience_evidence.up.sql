-- Persist experience evidence (V1-08).
ALTER TABLE experiences
    ADD COLUMN IF NOT EXISTS evidence JSONB NOT NULL DEFAULT '{}'::jsonb;
