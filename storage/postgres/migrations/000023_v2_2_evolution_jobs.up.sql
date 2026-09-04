-- V2.2-3: Persistent evolution dirty groups + jobs.

CREATE TABLE IF NOT EXISTS evolution_dirty_groups (
    tenant_id  TEXT NOT NULL,
    type       TEXT NOT NULL,
    scope      TEXT NOT NULL,
    scope_key  TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, type, scope, scope_key)
);

CREATE INDEX IF NOT EXISTS idx_evolution_dirty_updated
    ON evolution_dirty_groups (updated_at ASC);

CREATE TABLE IF NOT EXISTS evolution_jobs (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    type          TEXT NOT NULL,
    scope         TEXT NOT NULL,
    scope_key     TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    last_error    TEXT NOT NULL DEFAULT '',
    created_count INT  NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT evolution_jobs_status_check CHECK (
        status IN ('PENDING', 'PROCESSING', 'APPLIED', 'FAILED')
    ),
    CONSTRAINT evolution_jobs_family_unique UNIQUE (tenant_id, type, scope, scope_key)
);

CREATE INDEX IF NOT EXISTS idx_evolution_jobs_status_updated
    ON evolution_jobs (status, updated_at);
