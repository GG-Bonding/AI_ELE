-- V2.2-1: Pattern embeddings for semantic retrieval.
ALTER TABLE patterns
    ADD COLUMN IF NOT EXISTS embedding vector(1536);

COMMENT ON COLUMN patterns.embedding IS 'Optional; NULL for legacy rows. New generalizations embed trigger+content.';
