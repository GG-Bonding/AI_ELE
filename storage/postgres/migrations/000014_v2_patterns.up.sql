-- V2-7: Generalized Patterns derived from multiple experiences.

CREATE TABLE IF NOT EXISTS patterns (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    type          TEXT NOT NULL,
    scope         TEXT NOT NULL,
    scope_key     TEXT NOT NULL DEFAULT '',
    trigger_text  TEXT NOT NULL,
    content       TEXT NOT NULL,
    confidence    DOUBLE PRECISION NOT NULL,
    utility       DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    support_count INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT patterns_type_check CHECK (
        type IN ('EPISODIC', 'SEMANTIC', 'PROCEDURAL', 'FAILURE', 'CONSTRAINT', 'PREFERENCE')
    ),
    CONSTRAINT patterns_scope_check CHECK (
        scope IN ('GLOBAL', 'TENANT', 'TEAM', 'USER', 'AGENT', 'TOOL', 'TASK_TYPE')
    ),
    CONSTRAINT patterns_status_check CHECK (
        status IN ('CANDIDATE', 'ACTIVE', 'DEPRECATED', 'ARCHIVED')
    ),
    CONSTRAINT patterns_confidence_check CHECK (
        confidence >= 0 AND confidence <= 1
    ),
    CONSTRAINT patterns_utility_check CHECK (
        utility >= 0 AND utility <= 1
    ),
    CONSTRAINT patterns_support_count_check CHECK (
        support_count >= 0
    )
);

CREATE INDEX IF NOT EXISTS idx_patterns_tenant_status ON patterns (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_patterns_tenant_scope ON patterns (tenant_id, scope, scope_key);
CREATE INDEX IF NOT EXISTS idx_patterns_tenant_type ON patterns (tenant_id, type);

CREATE TABLE IF NOT EXISTS pattern_evidence (
    pattern_id    TEXT NOT NULL REFERENCES patterns(id) ON DELETE CASCADE,
    experience_id TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (pattern_id, experience_id)
);

CREATE INDEX IF NOT EXISTS idx_pattern_evidence_experience
    ON pattern_evidence (experience_id);
