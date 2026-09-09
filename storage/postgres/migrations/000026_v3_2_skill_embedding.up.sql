-- V3.2: SkillVersion embeddings for semantic retrieval (operator copy).

ALTER TABLE skill_versions
    ADD COLUMN IF NOT EXISTS embedding vector(1536);

COMMENT ON COLUMN skill_versions.embedding IS 'Optional; NULL for legacy rows. Embedded from name+description+tools+inputs.';
