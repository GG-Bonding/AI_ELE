-- V3-1: Executable Skill + immutable SkillVersion (runtime gated by skill_runtime.enabled).

CREATE TABLE IF NOT EXISTS skills (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    name               TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL,
    active_version_id  TEXT,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    CONSTRAINT skills_status_check CHECK (
        status IN (
            'CANDIDATE', 'VALIDATED', 'SHADOW', 'ACTIVE',
            'SUSPENDED', 'DEPRECATED', 'ARCHIVED'
        )
    ),
    CONSTRAINT skills_tenant_name_unique UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_skills_tenant_status
    ON skills (tenant_id, status);

CREATE TABLE IF NOT EXISTS skill_versions (
    id                  TEXT PRIMARY KEY,
    skill_id            TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    tenant_id           TEXT NOT NULL,
    version             BIGINT NOT NULL,
    pattern_id          TEXT,
    spec_json           JSONB NOT NULL,
    spec_yaml           TEXT NOT NULL DEFAULT '',
    spec_hash           TEXT NOT NULL,
    confidence          DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    utility             DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    status              TEXT NOT NULL,
    validation_status   TEXT NOT NULL DEFAULT 'PENDING',
    created_at          TIMESTAMPTZ NOT NULL,
    CONSTRAINT skill_versions_status_check CHECK (
        status IN (
            'CANDIDATE', 'VALIDATED', 'SHADOW', 'ACTIVE',
            'SUSPENDED', 'DEPRECATED', 'ARCHIVED'
        )
    ),
    CONSTRAINT skill_versions_validation_check CHECK (
        validation_status IN ('PENDING', 'PASSED', 'FAILED')
    ),
    CONSTRAINT skill_versions_confidence_check CHECK (
        confidence >= 0 AND confidence <= 1
    ),
    CONSTRAINT skill_versions_utility_check CHECK (
        utility >= 0 AND utility <= 1
    ),
    CONSTRAINT skill_versions_skill_version_unique UNIQUE (skill_id, version)
);

CREATE INDEX IF NOT EXISTS idx_skill_versions_tenant_skill
    ON skill_versions (tenant_id, skill_id);
CREATE INDEX IF NOT EXISTS idx_skill_versions_tenant_status
    ON skill_versions (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_skill_versions_spec_hash
    ON skill_versions (tenant_id, spec_hash);

-- Optional pointer integrity (deferred until version row exists).
ALTER TABLE skills
    DROP CONSTRAINT IF EXISTS skills_active_version_fk;
ALTER TABLE skills
    ADD CONSTRAINT skills_active_version_fk
    FOREIGN KEY (active_version_id) REFERENCES skill_versions(id)
    ON DELETE SET NULL;
