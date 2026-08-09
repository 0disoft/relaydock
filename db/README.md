# Database

`db/migrations/` is the only change history for the production database. `cmd/dbmigrate` embeds that SQL into the binary and applies it in order with a PostgreSQL advisory lock and one transaction per migration. Execution stops if the name or checksum of an applied migration has changed.

`db/schema.sql` is a snapshot of the **current final schema** after all migrations. Use it for sqlc type checking and fresh-install review, but do not execute it directly for production upgrades. Every new migration must update all three of the following in the same change:

1. `db/migrations/<version>_<name>.up.sql` and `.down.sql`
2. `db/schema.sql`
3. A repository or PostgreSQL integration test for the behavior

`db/queries/` is the sqlc generation contract. Repositories that need transactions, fencing, or dynamic queries on hot paths may use handwritten SQL, but both implementations must share the same schema semantics.

The CI `postgres-integration` job applies embedded migrations to an ephemeral PostgreSQL 18 database, then verifies runtime requests, provider attempts, usage events, transactional outbox behavior, and worker-lease fencing. Because this test runs `TRUNCATE`, use it only with a dedicated disposable database.

The Gateway does not own the money-platform ledger. This database stores only external settlement evidence such as `authorization_id`, `price_revision_id`, usage, and attempts. The money-platform owns customer balances and the capture ledger.
