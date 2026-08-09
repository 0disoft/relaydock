-- Durable expert worker leases, tenant scoping, and result provenance.

ALTER TABLE expert.context_packs
    ADD COLUMN IF NOT EXISTS tenant_id text NOT NULL DEFAULT '';

ALTER TABLE expert.consultations
    ADD COLUMN IF NOT EXISTS tenant_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS result_id text,
    ADD COLUMN IF NOT EXISTS failure_reason text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempt_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS lock_expires_at timestamptz;

ALTER TABLE expert.consultations
    DROP CONSTRAINT IF EXISTS expert_consultations_attempt_count_check;
ALTER TABLE expert.consultations
    ADD CONSTRAINT expert_consultations_attempt_count_check CHECK (attempt_count >= 0);

ALTER TABLE expert.consultation_results
    ALTER COLUMN consultation_id DROP NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'expert_consultations_result_id_fkey'
          AND conrelid = 'expert.consultations'::regclass
    ) THEN
        ALTER TABLE expert.consultations
            ADD CONSTRAINT expert_consultations_result_id_fkey
            FOREIGN KEY (result_id) REFERENCES expert.consultation_results(id);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS expert_consultations_claim_idx
    ON expert.consultations (route, state, available_at, created_at, id)
    WHERE state = 'queued';

CREATE INDEX IF NOT EXISTS expert_consultations_stale_claim_idx
    ON expert.consultations (lock_expires_at, id)
    WHERE state = 'running';

CREATE INDEX IF NOT EXISTS expert_consultations_scope_idx
    ON expert.consultations (project_id, tenant_id, updated_at DESC, id DESC);
