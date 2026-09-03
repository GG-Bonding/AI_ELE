-- V2.1-4: PatternLearningEvent exactly-once ledger.
DROP TABLE IF EXISTS pattern_reward_claims;

CREATE TABLE IF NOT EXISTS pattern_learning_events (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    feedback_id              TEXT NOT NULL,
    episode_id               TEXT NOT NULL DEFAULT '',
    pattern_id               TEXT NOT NULL,
    source_type              TEXT NOT NULL,
    source_learning_event_id TEXT NOT NULL DEFAULT '',
    normalized_reward        DOUBLE PRECISION NOT NULL,
    confidence               DOUBLE PRECISION NOT NULL,
    credit                   DOUBLE PRECISION NOT NULL,
    effective_reward         DOUBLE PRECISION NOT NULL,
    status                   TEXT NOT NULL DEFAULT 'PENDING',
    created_at               TIMESTAMPTZ NOT NULL,
    applied_at               TIMESTAMPTZ,
    CONSTRAINT pattern_learning_events_status_check CHECK (status IN ('PENDING', 'APPLIED', 'FAILED')),
    CONSTRAINT pattern_learning_events_source_check CHECK (
        source_type IN ('MEMBER_EXPERIENCE', 'PATTERN_USAGE', 'DIRECT')
    )
);

-- Member-derived events: unique per source experience learning event + pattern.
CREATE UNIQUE INDEX IF NOT EXISTS idx_pattern_learning_member_unique
    ON pattern_learning_events (tenant_id, source_learning_event_id, pattern_id)
    WHERE source_type = 'MEMBER_EXPERIENCE';

-- Usage / direct events: unique per feedback + pattern + source type.
CREATE UNIQUE INDEX IF NOT EXISTS idx_pattern_learning_feedback_source_unique
    ON pattern_learning_events (tenant_id, feedback_id, pattern_id, source_type)
    WHERE source_type IN ('PATTERN_USAGE', 'DIRECT');

CREATE INDEX IF NOT EXISTS idx_pattern_learning_events_tenant_feedback
    ON pattern_learning_events (tenant_id, feedback_id);
CREATE INDEX IF NOT EXISTS idx_pattern_learning_events_pending
    ON pattern_learning_events (tenant_id, status)
    WHERE status = 'PENDING';
