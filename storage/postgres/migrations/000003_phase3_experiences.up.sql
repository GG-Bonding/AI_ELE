-- Phase 3: Experience store with pgvector embeddings.

CREATE TABLE IF NOT EXISTS experiences (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    type              TEXT NOT NULL,
    scope             TEXT NOT NULL,
    scope_key         TEXT NOT NULL DEFAULT '',
    trigger_text      TEXT NOT NULL,
    content           TEXT NOT NULL,
    source_episode_id TEXT NOT NULL DEFAULT '',
    confidence        DOUBLE PRECISION NOT NULL,
    utility           DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    alpha             DOUBLE PRECISION NOT NULL DEFAULT 1,
    beta              DOUBLE PRECISION NOT NULL DEFAULT 1,
    success_count     BIGINT NOT NULL DEFAULT 0,
    failure_count     BIGINT NOT NULL DEFAULT 0,
    use_count         BIGINT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL,
    version           BIGINT NOT NULL DEFAULT 1,
    supersedes_id     TEXT,
    embedding         vector(1536) NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    last_used_at      TIMESTAMPTZ,
    CONSTRAINT experiences_type_check CHECK (
        type IN ('EPISODIC', 'SEMANTIC', 'PROCEDURAL', 'FAILURE', 'CONSTRAINT', 'PREFERENCE')
    ),
    CONSTRAINT experiences_scope_check CHECK (
        scope IN ('GLOBAL', 'TENANT', 'TEAM', 'USER', 'AGENT', 'TOOL', 'TASK_TYPE')
    ),
    CONSTRAINT experiences_status_check CHECK (
        status IN ('CANDIDATE', 'ACTIVE', 'DEPRECATED', 'BLOCKED', 'ARCHIVED')
    )
);

CREATE INDEX IF NOT EXISTS idx_experiences_tenant_status ON experiences (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_experiences_tenant_scope ON experiences (tenant_id, scope, scope_key);
CREATE INDEX IF NOT EXISTS idx_experiences_tenant_type ON experiences (tenant_id, type);
