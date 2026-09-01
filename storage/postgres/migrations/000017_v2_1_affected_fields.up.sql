-- V2.1: field-level provenance on ExperienceActionLink for ACTION_FIELD attribution.
ALTER TABLE experience_action_links
    ADD COLUMN IF NOT EXISTS affected_fields JSONB NOT NULL DEFAULT '[]'::jsonb;
