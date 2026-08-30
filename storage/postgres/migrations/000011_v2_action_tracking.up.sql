-- V2-1: Agent action tracking + experience→action links for attribution.

CREATE TABLE IF NOT EXISTS agent_actions (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    episode_id   TEXT NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    sequence     INT  NOT NULL,
    type         TEXT NOT NULL,
    tool_name    TEXT NOT NULL DEFAULT '',
    input        JSONB,
    output       JSONB,
    status       TEXT NOT NULL,
    attempt_id   TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    CONSTRAINT agent_actions_type_check CHECK (
        type IN ('TOOL_CALL', 'PLAN', 'DECISION', 'ANSWER', 'WORKFLOW_STEP')
    ),
    CONSTRAINT agent_actions_status_check CHECK (
        status IN ('RUNNING', 'SUCCESS', 'FAILED', 'SKIPPED')
    ),
    CONSTRAINT agent_actions_episode_sequence_unique UNIQUE (episode_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_agent_actions_tenant_episode
    ON agent_actions (tenant_id, episode_id);

CREATE TABLE IF NOT EXISTS experience_action_links (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    episode_id    TEXT NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    experience_id TEXT NOT NULL,
    action_id     TEXT NOT NULL REFERENCES agent_actions (id) ON DELETE CASCADE,
    influence     DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    evidence      TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT experience_action_links_influence_check CHECK (
        influence >= 0 AND influence <= 1
    ),
    CONSTRAINT experience_action_links_unique UNIQUE (tenant_id, experience_id, action_id)
);

CREATE INDEX IF NOT EXISTS idx_experience_action_links_tenant_episode
    ON experience_action_links (tenant_id, episode_id);
CREATE INDEX IF NOT EXISTS idx_experience_action_links_tenant_action
    ON experience_action_links (tenant_id, action_id);
CREATE INDEX IF NOT EXISTS idx_experience_action_links_tenant_experience
    ON experience_action_links (tenant_id, experience_id);
