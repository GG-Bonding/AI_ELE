-- Phase 7: Experience usage tracking for attribution.

CREATE TABLE IF NOT EXISTS experience_usages (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    episode_id          TEXT NOT NULL,
    experience_id       TEXT NOT NULL,
    retrieval_score     DOUBLE PRECISION NOT NULL DEFAULT 0,
    selection_decision  TEXT NOT NULL,
    final_score         DOUBLE PRECISION NOT NULL DEFAULT 0,
    used_at             TIMESTAMPTZ NOT NULL,
    CONSTRAINT experience_usages_decision_check CHECK (
        selection_decision IN ('KEEP', 'ABSTRACT', 'IGNORE', 'BLOCK')
    )
);

CREATE INDEX IF NOT EXISTS idx_experience_usages_tenant_episode
    ON experience_usages (tenant_id, episode_id);
CREATE INDEX IF NOT EXISTS idx_experience_usages_tenant_experience
    ON experience_usages (tenant_id, experience_id);
