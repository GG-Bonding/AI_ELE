-- Phase 6: Feedback pipeline raw signal storage (operator copy).

CREATE TABLE IF NOT EXISTS feedbacks (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    episode_id  TEXT NOT NULL,
    source      TEXT NOT NULL,
    signal      TEXT NOT NULL DEFAULT '',
    reward      DOUBLE PRECISION NOT NULL,
    confidence  DOUBLE PRECISION NOT NULL,
    evidence    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    CONSTRAINT feedbacks_source_check CHECK (
        source IN (
            'USER_EXPLICIT',
            'USER_IMPLICIT',
            'TOOL',
            'BUSINESS',
            'HUMAN_REVIEW',
            'LLM_JUDGE'
        )
    ),
    CONSTRAINT feedbacks_reward_check CHECK (reward >= -1 AND reward <= 1),
    CONSTRAINT feedbacks_confidence_check CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE INDEX IF NOT EXISTS idx_feedbacks_tenant_episode ON feedbacks (tenant_id, episode_id);
CREATE INDEX IF NOT EXISTS idx_feedbacks_tenant_created ON feedbacks (tenant_id, created_at DESC);
