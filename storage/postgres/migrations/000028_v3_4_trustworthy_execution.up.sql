-- V3.4: Trustworthy real-world execution (attempts, fencing, UNKNOWN).

ALTER TABLE skill_executions
    ADD COLUMN IF NOT EXISTS lease_epoch BIGINT NOT NULL DEFAULT 0;

ALTER TABLE skill_step_executions
    ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS operation_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_epoch BIGINT NOT NULL DEFAULT 0;

-- Expand step status constraint for UNKNOWN_OUTCOME.
ALTER TABLE skill_step_executions DROP CONSTRAINT IF EXISTS skill_step_executions_status_check;
ALTER TABLE skill_step_executions
    ADD CONSTRAINT skill_step_executions_status_check CHECK (
        status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'SKIPPED', 'SHADOWED', 'UNKNOWN_OUTCOME')
    );

-- Expand execution status for reconciliation hold.
ALTER TABLE skill_executions DROP CONSTRAINT IF EXISTS skill_executions_status_check;
ALTER TABLE skill_executions
    ADD CONSTRAINT skill_executions_status_check CHECK (
        status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'WAITING_APPROVAL', 'DENIED', 'NEEDS_RECONCILIATION')
    );

CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_step_operation_attempt
    ON skill_step_executions (tenant_id, execution_id, operation_key, attempt)
    WHERE operation_key <> '';

COMMENT ON COLUMN skill_executions.lease_epoch IS 'Fencing token; increments on each lease claim.';
COMMENT ON COLUMN skill_step_executions.operation_key IS 'Stable logical op key: execution_id:step_id (no attempt).';
COMMENT ON COLUMN skill_step_executions.attempt IS 'Transport attempt counter for diagnostics only.';
