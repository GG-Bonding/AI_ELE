-- V2.2-2: Context provenance snapshots + action binding (operator copy).

CREATE TABLE IF NOT EXISTS context_snapshots (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    episode_id      TEXT NOT NULL DEFAULT '',
    agent_id        TEXT NOT NULL DEFAULT '',
    user_id         TEXT NOT NULL DEFAULT '',
    task            TEXT NOT NULL,
    experience_ids  JSONB NOT NULL DEFAULT '[]'::jsonb,
    pattern_ids     JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_context_snapshots_tenant_episode
    ON context_snapshots (tenant_id, episode_id);

ALTER TABLE agent_actions
    ADD COLUMN IF NOT EXISTS context_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_agent_actions_tenant_context
    ON agent_actions (tenant_id, context_id)
    WHERE context_id <> '';

CREATE TABLE IF NOT EXISTS pattern_action_links (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    episode_id TEXT NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    pattern_id TEXT NOT NULL,
    action_id  TEXT NOT NULL REFERENCES agent_actions (id) ON DELETE CASCADE,
    influence  DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    evidence   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pattern_action_links_influence_check CHECK (
        influence >= 0 AND influence <= 1
    ),
    CONSTRAINT pattern_action_links_unique UNIQUE (tenant_id, pattern_id, action_id)
);

CREATE INDEX IF NOT EXISTS idx_pattern_action_links_tenant_episode
    ON pattern_action_links (tenant_id, episode_id);
CREATE INDEX IF NOT EXISTS idx_pattern_action_links_tenant_action
    ON pattern_action_links (tenant_id, action_id);
