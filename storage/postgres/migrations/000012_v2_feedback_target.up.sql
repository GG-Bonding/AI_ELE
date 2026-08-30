-- V2-2: Feedback targeting (action / field / experience / tool).

ALTER TABLE feedbacks
    ADD COLUMN IF NOT EXISTS target JSONB;

CREATE INDEX IF NOT EXISTS idx_feedbacks_tenant_target_action
    ON feedbacks (tenant_id, ((target->>'action_id')))
    WHERE target IS NOT NULL AND target->>'action_id' IS NOT NULL AND target->>'action_id' <> '';
