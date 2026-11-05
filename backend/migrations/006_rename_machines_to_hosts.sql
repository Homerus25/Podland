-- Rename the domain term machine -> host in the physical schema.
--
-- 001_initial_schema.sql is deliberately left untouched: it keeps creating the
-- old tables, columns, indexes, and trigger, so a fresh database walks the same
-- path an existing one does. This migration then renames them in place, which
-- keeps every registered host, its encrypted SSH key, and its access grants.
--
-- ALTER TABLE ... RENAME TO auto-rewrites the foreign key in machine_access and
-- the trigger body, but index and trigger *names* keep the old vocabulary, so
-- they are dropped and recreated under the new names.

DROP TRIGGER IF EXISTS update_machine_updated_at;
DROP INDEX IF EXISTS idx_machines_owner_id;
DROP INDEX IF EXISTS idx_machines_name;
DROP INDEX IF EXISTS idx_machine_access_machine_id;
DROP INDEX IF EXISTS idx_machine_access_user_id;

ALTER TABLE machines RENAME TO hosts;
ALTER TABLE machine_access RENAME TO host_access;
ALTER TABLE host_access RENAME COLUMN machine_id TO host_id;

CREATE INDEX IF NOT EXISTS idx_hosts_owner_id ON hosts(owner_id);
CREATE INDEX IF NOT EXISTS idx_hosts_name ON hosts(name);
CREATE INDEX IF NOT EXISTS idx_host_access_host_id ON host_access(host_id);
CREATE INDEX IF NOT EXISTS idx_host_access_user_id ON host_access(user_id);

CREATE TRIGGER IF NOT EXISTS update_host_updated_at
AFTER UPDATE ON hosts
FOR EACH ROW
BEGIN
    UPDATE hosts SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
