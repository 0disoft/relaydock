-- Current schema snapshot after every embedded migration. Use cmd/dbmigrate for upgrades.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS control;
CREATE SCHEMA IF NOT EXISTS runtime;
CREATE SCHEMA IF NOT EXISTS expert;
CREATE SCHEMA IF NOT EXISTS outbox;

CREATE TABLE IF NOT EXISTS control.organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS control.projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES control.organizations(id),
    slug text NOT NULL,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

CREATE TABLE IF NOT EXISTS control.virtual_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES control.projects(id),
    public_id text NOT NULL UNIQUE,
    secret_digest bytea NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    allowed_models text[] NOT NULL DEFAULT '{}',
    expires_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS control.provider_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES control.organizations(id),
    provider text NOT NULL,
    credential_reference text NOT NULL,
    region text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'unconfigured' CHECK (state IN ('unconfigured', 'healthy', 'degraded', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS control.model_routes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES control.projects(id),
    virtual_model text NOT NULL,
    policy jsonb NOT NULL,
    revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, virtual_model, revision)
);

CREATE TABLE IF NOT EXISTS control.runtime_snapshots (
    revision bigint PRIMARY KEY,
    payload bytea NOT NULL,
    signature bytea NOT NULL,
    generated_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime.requests (
    id text PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES control.projects(id),
    virtual_key_id uuid NOT NULL REFERENCES control.virtual_keys(id),
    ingress_protocol text NOT NULL,
    virtual_model text NOT NULL,
    price_revision_id text NOT NULL,
    authorization_id text,
    state text NOT NULL CHECK (state IN ('received', 'authorized', 'running', 'completed', 'failed', 'cancelled')),
    client_request_id text NOT NULL DEFAULT '',
    tenant_id text NOT NULL DEFAULT '',
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cache_read_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    cache_write_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    reasoning_tokens bigint NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
    error_code text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    first_semantic_event_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE TABLE IF NOT EXISTS runtime.provider_attempts (
    id text PRIMARY KEY,
    request_id text NOT NULL REFERENCES runtime.requests(id),
    provider_connection_id uuid REFERENCES control.provider_connections(id),
    upstream_model text NOT NULL,
    attempt_number integer NOT NULL,
    state text NOT NULL CHECK (state IN ('reserved', 'running', 'completed', 'failed', 'cancelled')),
    provider text NOT NULL DEFAULT '',
    account_id text NOT NULL DEFAULT '',
    protocol text NOT NULL DEFAULT '',
    committed boolean NOT NULL DEFAULT false,
    error_code text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (request_id, attempt_number)
);

CREATE TABLE IF NOT EXISTS runtime.usage_events (
    id text PRIMARY KEY,
    request_id text NOT NULL REFERENCES runtime.requests(id),
    attempt_id text NOT NULL REFERENCES runtime.provider_attempts(id),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cache_read_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    cache_write_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    reasoning_tokens bigint NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
    provider_report jsonb,
    local_estimate jsonb,
    price_revision_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (request_id, attempt_id)
);

CREATE TABLE IF NOT EXISTS expert.context_packs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL DEFAULT '',
    project_id uuid NOT NULL REFERENCES control.projects(id),
    repository_revision text NOT NULL,
    working_tree_digest text NOT NULL,
    manifest jsonb NOT NULL,
    object_key text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS expert.consultations (
    id text PRIMARY KEY,
    tenant_id text NOT NULL DEFAULT '',
    project_id uuid NOT NULL REFERENCES control.projects(id),
    context_pack_id text NOT NULL REFERENCES expert.context_packs(id),
    objective text NOT NULL,
    task_type text NOT NULL,
    route text NOT NULL,
    state text NOT NULL CHECK (state IN ('created', 'context_pending', 'approval_pending', 'queued', 'running', 'result_pending', 'completed', 'failed', 'cancelled', 'expired')),
    maximum_cost_minor bigint NOT NULL DEFAULT 0 CHECK (maximum_cost_minor >= 0),
    authorization_id text,
    idempotency_key text NOT NULL,
    request_fingerprint bytea NOT NULL,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_by text,
    locked_at timestamptz,
    lock_expires_at timestamptz,
    result_id text,
    failure_reason text NOT NULL DEFAULT '',
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS expert.consultation_results (
    id text PRIMARY KEY,
    consultation_id text UNIQUE REFERENCES expert.consultations(id),
    structured_result jsonb NOT NULL,
    model_attestation text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

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

CREATE TABLE IF NOT EXISTS outbox.events (
    id text PRIMARY KEY,
    topic text NOT NULL,
    aggregate_id text NOT NULL,
    payload jsonb NOT NULL,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_by text,
    locked_at timestamptz,
    lock_expires_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text NOT NULL DEFAULT '',
    published_at timestamptz,
    dead_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (published_at IS NULL OR dead_at IS NULL)
);

CREATE INDEX IF NOT EXISTS runtime_requests_project_created_idx
    ON runtime.requests (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS runtime_requests_client_request_idx
    ON runtime.requests (project_id, client_request_id, created_at DESC)
    WHERE client_request_id <> '';
CREATE INDEX IF NOT EXISTS runtime_attempts_request_state_idx
    ON runtime.provider_attempts (request_id, state, attempt_number);
CREATE INDEX IF NOT EXISTS expert_consultations_queue_idx
    ON expert.consultations (state, available_at, created_at);
CREATE INDEX IF NOT EXISTS expert_consultations_claim_idx
    ON expert.consultations (route, state, available_at, created_at, id)
    WHERE state = 'queued';
CREATE INDEX IF NOT EXISTS expert_consultations_stale_claim_idx
    ON expert.consultations (lock_expires_at, id)
    WHERE state = 'running';
CREATE INDEX IF NOT EXISTS expert_consultations_scope_idx
    ON expert.consultations (project_id, tenant_id, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS outbox_events_queue_idx
    ON outbox.events (available_at, created_at, id)
    WHERE published_at IS NULL AND dead_at IS NULL;
CREATE INDEX IF NOT EXISTS outbox_events_stale_lock_idx
    ON outbox.events (lock_expires_at, id)
    WHERE published_at IS NULL AND dead_at IS NULL AND locked_by IS NOT NULL;
CREATE INDEX IF NOT EXISTS outbox_events_dead_idx
    ON outbox.events (dead_at, id)
    WHERE dead_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS outbox_events_published_idx
    ON outbox.events (published_at, id)
    WHERE published_at IS NOT NULL;
