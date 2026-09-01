-- V2.1-3: PatternUsage ledger for patterns that entered agent context.
CREATE TABLE IF NOT EXISTS pattern_usages (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    episode_id       TEXT NOT NULL,
    pattern_id       TEXT NOT NULL,
    retrieval_score  DOUBLE PRECISION NOT NULL DEFAULT 0,
    final_score      DOUBLE PRECISION NOT NULL DEFAULT 0,
    used_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pattern_usages_tenant_episode
    ON pattern_usages (tenant_id, episode_id);
CREATE INDEX IF NOT EXISTS idx_pattern_usages_tenant_pattern
    ON pattern_usages (tenant_id, pattern_id);

-- Idempotent marks for episode-level PatternUsage rewards (full PatternLearningEvent in V2.1-4).
CREATE TABLE IF NOT EXISTS pattern_reward_claims (
    tenant_id   TEXT NOT NULL,
    feedback_id TEXT NOT NULL,
    pattern_id  TEXT NOT NULL,
    reward      DOUBLE PRECISION NOT NULL,
    confidence  DOUBLE PRECISION NOT NULL,
    credit      DOUBLE PRECISION NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, feedback_id, pattern_id)
);
