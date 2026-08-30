-- Allow COMPRESS selection decision (V1 rename of ABSTRACT).

ALTER TABLE experience_usages DROP CONSTRAINT IF EXISTS experience_usages_selection_decision_check;
ALTER TABLE experience_usages ADD CONSTRAINT experience_usages_selection_decision_check
    CHECK (selection_decision IN ('KEEP', 'ABSTRACT', 'COMPRESS', 'IGNORE', 'BLOCK'));
