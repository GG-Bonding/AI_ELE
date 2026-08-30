-- V2-5: Experience relations for conflict / supersession / derivation.

CREATE TABLE IF NOT EXISTS experience_relations (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    from_experience_id   TEXT NOT NULL,
    to_experience_id     TEXT NOT NULL,
    type                 TEXT NOT NULL,
    confidence           DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    reason               TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL,
    CONSTRAINT experience_relations_type_check CHECK (
        type IN ('DUPLICATE', 'SUPPORTS', 'CONFLICTS', 'SUPERSEDES', 'DERIVED_FROM')
    ),
    CONSTRAINT experience_relations_confidence_check CHECK (
        confidence >= 0 AND confidence <= 1
    ),
    CONSTRAINT experience_relations_endpoints_distinct CHECK (
        from_experience_id <> to_experience_id
    ),
    CONSTRAINT experience_relations_unique UNIQUE (tenant_id, from_experience_id, to_experience_id, type)
);

CREATE INDEX IF NOT EXISTS idx_experience_relations_tenant_from
    ON experience_relations (tenant_id, from_experience_id);
CREATE INDEX IF NOT EXISTS idx_experience_relations_tenant_to
    ON experience_relations (tenant_id, to_experience_id);
CREATE INDEX IF NOT EXISTS idx_experience_relations_tenant_type
    ON experience_relations (tenant_id, type);
