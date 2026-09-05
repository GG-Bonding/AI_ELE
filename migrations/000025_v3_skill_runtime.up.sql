-- V3-3..V3-7: execution ledger, learning events, version beta counters.

ALTER TABLE skill_versions
    ADD COLUMN IF NOT EXISTS alpha DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS beta DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS success_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS failure_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS shadow_successes INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS shadow_failures INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS skill_executions (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    episode_id        TEXT,
    skill_id          TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    skill_version_id  TEXT NOT NULL REFERENCES skill_versions(id) ON DELETE CASCADE,
    mode              TEXT NOT NULL,
    status            TEXT NOT NULL,
    idempotency_key   TEXT,
    inputs            JSONB NOT NULL DEFAULT '{}',
    outputs           JSONB NOT NULL DEFAULT '{}',
    error_code        TEXT NOT NULL DEFAULT '',
    error_message     TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ NOT NULL,
    completed_at      TIMESTAMPTZ,
    CONSTRAINT skill_executions_mode_check CHECK (mode IN ('SHADOW', 'LIVE')),
    CONSTRAINT skill_executions_status_check CHECK (
        status IN (
            'PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED',
            'CANCELLED', 'WAITING_APPROVAL', 'DENIED'
        )
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_executions_idempotency
    ON skill_executions (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_skill_executions_tenant_version
    ON skill_executions (tenant_id, skill_version_id, mode, status);

CREATE TABLE IF NOT EXISTS skill_step_executions (
    id            TEXT PRIMARY KEY,
    execution_id  TEXT NOT NULL REFERENCES skill_executions(id) ON DELETE CASCADE,
    tenant_id     TEXT NOT NULL,
    step_id       TEXT NOT NULL,
    tool          TEXT NOT NULL,
    input         JSONB NOT NULL DEFAULT '{}',
    output        JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL,
    error_code    TEXT NOT NULL DEFAULT '',
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    sequence      INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT skill_step_executions_status_check CHECK (
        status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'SKIPPED', 'SHADOWED')
    )
);

CREATE INDEX IF NOT EXISTS idx_skill_step_executions_exec
    ON skill_step_executions (tenant_id, execution_id, sequence);

CREATE TABLE IF NOT EXISTS skill_approval_requests (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    execution_id  TEXT NOT NULL REFERENCES skill_executions(id) ON DELETE CASCADE,
    skill_id      TEXT NOT NULL,
    status        TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL,
    resolved_at   TIMESTAMPTZ,
    CONSTRAINT skill_approval_status_check CHECK (
        status IN ('PENDING', 'APPROVED', 'REJECTED')
    )
);

CREATE TABLE IF NOT EXISTS skill_learning_events (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    skill_id           TEXT NOT NULL,
    skill_version_id   TEXT NOT NULL REFERENCES skill_versions(id) ON DELETE CASCADE,
    execution_id       TEXT,
    feedback_id        TEXT NOT NULL,
    reward             DOUBLE PRECISION NOT NULL,
    confidence         DOUBLE PRECISION NOT NULL,
    credit             DOUBLE PRECISION NOT NULL DEFAULT 1,
    status             TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL,
    applied_at         TIMESTAMPTZ,
    CONSTRAINT skill_learning_events_status_check CHECK (
        status IN ('PENDING', 'APPLIED', 'FAILED')
    ),
    CONSTRAINT skill_learning_events_feedback_version_unique
        UNIQUE (tenant_id, feedback_id, skill_version_id)
);

CREATE INDEX IF NOT EXISTS idx_skill_learning_events_pending
    ON skill_learning_events (tenant_id, status);
