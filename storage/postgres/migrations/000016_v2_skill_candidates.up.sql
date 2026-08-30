-- V2-9: Skill Candidates derived from Patterns (description only; no auto-exec).

CREATE TABLE IF NOT EXISTS skill_candidates (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    pattern_id  TEXT NOT NULL REFERENCES patterns(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    spec_yaml   TEXT NOT NULL,
    confidence  DOUBLE PRECISION NOT NULL,
    utility     DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    status      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    CONSTRAINT skill_candidates_status_check CHECK (
        status IN ('CANDIDATE', 'DEPRECATED', 'ARCHIVED')
    ),
    CONSTRAINT skill_candidates_confidence_check CHECK (
        confidence >= 0 AND confidence <= 1
    ),
    CONSTRAINT skill_candidates_utility_check CHECK (
        utility >= 0 AND utility <= 1
    ),
    CONSTRAINT skill_candidates_pattern_unique UNIQUE (tenant_id, pattern_id)
);

CREATE INDEX IF NOT EXISTS idx_skill_candidates_tenant_status
    ON skill_candidates (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_skill_candidates_tenant_pattern
    ON skill_candidates (tenant_id, pattern_id);
