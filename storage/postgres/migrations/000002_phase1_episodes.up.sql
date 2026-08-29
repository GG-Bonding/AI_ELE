-- Phase 1: Episode / Attempt / Outcome lifecycle tables.

CREATE TABLE IF NOT EXISTS episodes (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    agent_id     TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    task_type    TEXT NOT NULL DEFAULT '',
    goal         TEXT NOT NULL,
    input        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    CONSTRAINT episodes_status_check CHECK (
        status IN ('RUNNING', 'SUCCESS', 'PARTIAL', 'FAILED', 'CANCELLED')
    )
);

CREATE INDEX IF NOT EXISTS idx_episodes_tenant_id ON episodes (tenant_id);
CREATE INDEX IF NOT EXISTS idx_episodes_tenant_agent ON episodes (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_episodes_tenant_status ON episodes (tenant_id, status);

CREATE TABLE IF NOT EXISTS attempts (
    id            TEXT PRIMARY KEY,
    episode_id    TEXT NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    tenant_id     TEXT NOT NULL,
    sequence      INT  NOT NULL,
    hypothesis    TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL DEFAULT '',
    tool_name     TEXT NOT NULL DEFAULT '',
    input         JSONB,
    output        JSONB,
    status        TEXT NOT NULL,
    error_code    TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL,
    completed_at  TIMESTAMPTZ,
    CONSTRAINT attempts_status_check CHECK (
        status IN ('RUNNING', 'SUCCESS', 'FAILED', 'SKIPPED')
    ),
    CONSTRAINT attempts_episode_sequence_unique UNIQUE (episode_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_attempts_tenant_episode ON attempts (tenant_id, episode_id);

CREATE TABLE IF NOT EXISTS outcomes (
    id         TEXT PRIMARY KEY,
    episode_id TEXT NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    tenant_id  TEXT NOT NULL,
    status     TEXT NOT NULL,
    result     JSONB,
    verified   BOOLEAN NOT NULL DEFAULT FALSE,
    verifier   TEXT NOT NULL DEFAULT '',
    metrics    JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT outcomes_episode_unique UNIQUE (episode_id)
);

CREATE INDEX IF NOT EXISTS idx_outcomes_tenant_episode ON outcomes (tenant_id, episode_id);
