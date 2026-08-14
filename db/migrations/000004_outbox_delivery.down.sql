/* llmnav/1 module
id=relaydock.migration.outbox-delivery.down
role=Remove recoverable lease and dead-letter state while restoring the initial pending-event queue index.
owns=outbox delivery rollback|initial queue index restoration
excludes=outbox event deletion|published event cleanup
search=rollback outbox delivery|remove dead letter columns|restore outbox queue index
invariant=Delivery-specific indexes are removed before their backing columns.
invariant=The restored queue index continues to exclude published events.
stability=contract
*/

ALTER TABLE outbox.events
    DROP CONSTRAINT IF EXISTS outbox_events_terminal_state_check;

DROP INDEX IF EXISTS outbox.outbox_events_published_idx;
DROP INDEX IF EXISTS outbox.outbox_events_dead_idx;
DROP INDEX IF EXISTS outbox.outbox_events_stale_lock_idx;
DROP INDEX IF EXISTS outbox.outbox_events_claim_idx;

CREATE INDEX IF NOT EXISTS outbox_events_queue_idx
    ON outbox.events (available_at, created_at)
    WHERE published_at IS NULL;

ALTER TABLE outbox.events
    DROP COLUMN IF EXISTS dead_at,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS lock_expires_at;
