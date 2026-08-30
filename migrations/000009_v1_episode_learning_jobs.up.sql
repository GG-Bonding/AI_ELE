CREATE TABLE IF NOT EXISTS episode_learning_jobs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  episode_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('PENDING','PROCESSING','APPLIED','FAILED')),
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, episode_id)
);
CREATE INDEX IF NOT EXISTS idx_episode_learning_jobs_status ON episode_learning_jobs (tenant_id, status);
