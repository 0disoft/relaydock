/* llmnav/1 module
id=relaydock.migration.outbox-delivery.up
role=Add recoverable outbox lease expiry, bounded failure state, dead-letter state, and worker-oriented partial indexes.
owns=outbox delivery schema|outbox lease recovery index|dead-letter persistence
excludes=delivery retry algorithm|event publisher
search=add outbox lease expiry|dead letter migration|outbox worker indexes
invariant=An event cannot be both published and dead-lettered.
invariant=Claim and stale-lock indexes exclude terminal events.
stability=contract
*/

-- Add recoverable worker leases and dead-letter metadata to the transactional
-- outbox. locked_at alone cannot distinguish a live worker from an abandoned
-- claim after a process crash.

ALTER TABLE outbox.events
    ADD COLUMN IF NOT EXISTS lock_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS last_error text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dead_at timestamptz;

DROP INDEX IF EXISTS outbox.outbox_events_queue_idx;

CREATE INDEX IF NOT EXISTS outbox_events_claim_idx
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

ALTER TABLE outbox.events
    DROP CONSTRAINT IF EXISTS outbox_events_terminal_state_check,
    ADD CONSTRAINT outbox_events_terminal_state_check
        CHECK (published_at IS NULL OR dead_at IS NULL);
