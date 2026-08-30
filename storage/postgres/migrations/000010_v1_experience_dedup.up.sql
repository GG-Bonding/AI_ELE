ALTER TABLE experiences ADD COLUMN IF NOT EXISTS dedup_key TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_experiences_tenant_episode_dedup
  ON experiences (tenant_id, source_episode_id, dedup_key)
  WHERE dedup_key <> '' AND source_episode_id <> '';
