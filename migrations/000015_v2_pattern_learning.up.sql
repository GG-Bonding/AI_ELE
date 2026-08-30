-- V2-8: Patterns carry Beta utility (alpha/beta) like experiences (operator copy).

ALTER TABLE patterns
    ADD COLUMN IF NOT EXISTS alpha DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS beta DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS success_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS failure_count BIGINT NOT NULL DEFAULT 0;

UPDATE patterns
SET
    alpha = CASE
        WHEN utility <= 0 THEN 1
        WHEN utility >= 1 THEN 9
        WHEN utility >= 0.5 THEN utility / (1 - utility)
        ELSE 1
    END,
    beta = CASE
        WHEN utility <= 0 THEN 9
        WHEN utility >= 1 THEN 1
        WHEN utility >= 0.5 THEN 1
        ELSE (1 - utility) / utility
    END
WHERE alpha = 1 AND beta = 1 AND utility <> 0.5;
