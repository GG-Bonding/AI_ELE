-- V1 hardening: feedback idempotency + incremental learning events.

ALTER TABLE feedbacks
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_feedbacks_tenant_idempotency
    ON feedbacks (tenant_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS learning_events (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    feedback_id        TEXT NOT NULL,
    episode_id         TEXT NOT NULL,
    experience_id      TEXT NOT NULL,
    normalized_reward  DOUBLE PRECISION NOT NULL,
    confidence         DOUBLE PRECISION NOT NULL,
    credit             DOUBLE PRECISION NOT NULL,
    effective_reward   DOUBLE PRECISION NOT NULL,
    status             TEXT NOT NULL DEFAULT 'PENDING',
    created_at         TIMESTAMPTZ NOT NULL,
    applied_at         TIMESTAMPTZ,
    CONSTRAINT learning_events_status_check CHECK (status IN ('PENDING', 'APPLIED', 'FAILED')),
    CONSTRAINT learning_events_feedback_experience_unique UNIQUE (tenant_id, feedback_id, experience_id)
);

CREATE INDEX IF NOT EXISTS idx_learning_events_tenant_feedback
    ON learning_events (tenant_id, feedback_id);
CREATE INDEX IF NOT EXISTS idx_learning_events_pending
    ON learning_events (tenant_id, status)
    WHERE status = 'PENDING';
