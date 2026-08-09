DROP INDEX IF EXISTS expert.expert_consultations_scope_idx;
DROP INDEX IF EXISTS expert.expert_consultations_stale_claim_idx;
DROP INDEX IF EXISTS expert.expert_consultations_claim_idx;

ALTER TABLE expert.consultations
    DROP CONSTRAINT IF EXISTS expert_consultations_result_id_fkey,
    DROP CONSTRAINT IF EXISTS expert_consultations_attempt_count_check,
    DROP COLUMN IF EXISTS lock_expires_at,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS result_id,
    DROP COLUMN IF EXISTS tenant_id;

ALTER TABLE expert.consultation_results
    ALTER COLUMN consultation_id SET NOT NULL;

ALTER TABLE expert.context_packs
    DROP COLUMN IF EXISTS tenant_id;
