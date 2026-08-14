/* llmnav/1 module
id=relaydock.migration.initial.down
role=Remove every application schema created by the initial RelayDock migration during an explicit full rollback.
owns=initial schema rollback|application schema removal
excludes=public migration ledger|database backup
search=rollback initial RelayDock schema|drop application schemas|initial migration down
invariant=Rollback removes outbox, expert, runtime, and control schemas as one ordered migration step.
invariant=The public schema and migration runner ledger remain outside this rollback file.
stability=contract
*/

DROP SCHEMA IF EXISTS outbox CASCADE;
DROP SCHEMA IF EXISTS expert CASCADE;
DROP SCHEMA IF EXISTS runtime CASCADE;
DROP SCHEMA IF EXISTS control CASCADE;
