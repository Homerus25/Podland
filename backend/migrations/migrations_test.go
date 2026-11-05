package migrations

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pre-006 schema speaks the retired domain vocabulary. This test has to
// name the old objects to prove the upgrade path, so it is one of the
// declared exceptions to the backend-wide "no retired vocabulary" search
// (ticket #455, architecture decision §21) — alongside the two migration
// files themselves.
const (
	retiredTerm           = "machine"
	preRenameHostsTable   = retiredTerm + "s"
	preRenameAccessTable  = retiredTerm + "_access"
	preRenameHostIDColumn = retiredTerm + "_id"
)

// TestRun_UpgradesPreviousSchemaKeepingHostsAndGrants is the migration's
// proof against a database created at the previous schema: a deployment that
// already ran 001-005 keeps its registered hosts, their encrypted SSH keys,
// and their access grants when 006 lands.
func TestRun_UpgradesPreviousSchemaKeepingHostsAndGrants(t *testing.T) {
	// The runner resolves migration files relative to the backend module
	// root, so the test runs from there.
	t.Chdir("..")

	db := openTestDB(t)
	runMigrationsThrough(t, db, "005")

	const (
		ownerID   = "user-owner"
		granteeID = "user-grantee"
		hostID    = "host-1"
	)
	// A key blob with bytes that survive no lossy round trip: NULs, high
	// bytes, and a newline.
	keyBytes := []byte{0x00, 0x01, 0xfe, 0xff, 0x7f, 0x80, 0x0a, 0x0d, 0x00}

	_, err := db.Exec(`INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)`, ownerID, "owner", "hash")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)`, granteeID, "grantee", "hash")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO `+preRenameHostsTable+` (id, name, owner_id, description, hostname, port, ssh_username, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hostID, "prod-host", ownerID, "kept across the rename", "prod.example.com", 2222,
		"podland", keyBytes, "SHA256:abc123", "/run/podman/podman.sock")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO `+preRenameAccessTable+` (`+preRenameHostIDColumn+`, user_id, permission) VALUES (?, ?, ?)`,
		hostID, granteeID, "write")
	require.NoError(t, err)

	require.NoError(t, Run(db), "the upgrade migration must apply cleanly to a previous-schema database")

	// The host row survives under the new table with every column intact.
	var (
		name, hostname, fingerprint, socketPath string
		port                                    int
		gotKey                                  []byte
	)
	require.NoError(t, db.QueryRow(
		`SELECT name, hostname, port, ssh_key_encrypted, ssh_key_fingerprint, podman_socket_path FROM hosts WHERE id = ?`,
		hostID,
	).Scan(&name, &hostname, &port, &gotKey, &fingerprint, &socketPath))
	assert.Equal(t, "prod-host", name)
	assert.Equal(t, "prod.example.com", hostname)
	assert.Equal(t, 2222, port)
	assert.Equal(t, "SHA256:abc123", fingerprint)
	assert.Equal(t, "/run/podman/podman.sock", socketPath)
	assert.Equal(t, keyBytes, gotKey, "the encrypted SSH key bytes must be byte-identical after the rename")

	// The access grant survives under the new table and column.
	var permission string
	require.NoError(t, db.QueryRow(
		`SELECT permission FROM host_access WHERE host_id = ? AND user_id = ?`, hostID, granteeID,
	).Scan(&permission))
	assert.Equal(t, "write", permission)

	// The grant's foreign key was rewritten to point at the renamed table.
	var fkTable string
	require.NoError(t, db.QueryRow(
		`SELECT "table" FROM pragma_foreign_key_list('host_access') WHERE "from" = 'host_id'`,
	).Scan(&fkTable))
	assert.Equal(t, "hosts", fkTable)

	// The indexes and the trigger were rebuilt under the new vocabulary.
	for _, index := range []string{
		"idx_hosts_owner_id",
		"idx_hosts_name",
		"idx_host_access_host_id",
		"idx_host_access_user_id",
	} {
		assert.True(t, sqliteObjectExists(t, db, "index", index), "index %s must exist after the rename", index)
	}
	assert.True(t, sqliteObjectExists(t, db, "trigger", "update_host_updated_at"),
		"the updated_at trigger must be recreated under the new name")

	// A stale trigger body still naming the old table would make this update
	// fail outright, so a clean update proves the body was rewritten.
	_, err = db.Exec(`UPDATE hosts SET name = ? WHERE id = ?`, "renamed-host", hostID)
	require.NoError(t, err, "the recreated trigger must resolve against the renamed table")

	// Nothing in the schema still carries the retired vocabulary.
	assert.Empty(t, retiredSchemaObjects(t, db),
		"no schema object may keep the retired vocabulary after the upgrade")
}

// TestRun_FreshDatabaseEndsOnHostsSchema pins the other half of the upgrade
// path: a brand-new database walks 001 (old schema) then 006 (rename) and must
// land on the same schema an upgraded one does.
func TestRun_FreshDatabaseEndsOnHostsSchema(t *testing.T) {
	t.Chdir("..")

	db := openTestDB(t)
	require.NoError(t, Run(db))

	assert.True(t, sqliteObjectExists(t, db, "table", "hosts"), "a fresh database must end on the hosts table")
	assert.True(t, sqliteObjectExists(t, db, "table", "host_access"), "a fresh database must end on the host_access table")
	assert.Empty(t, retiredSchemaObjects(t, db),
		"no schema object may keep the retired vocabulary after a fresh migration run")
}

// openTestDB opens a throwaway database through the production connection
// factory, so the migration is proven against the pragmas the app runs with
// (WAL, foreign keys on, busy timeout).
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := models.NewDatabase(filepath.Join(t.TempDir(), "podland.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// runMigrationsThrough applies the migration files up to and including the
// given numeric prefix, recording them exactly as Run does, so the database is
// left at the previous schema and the subsequent Run applies only the later
// files. The runner has no "up to N" API; this is the test-local stand-in that
// makes the upgrade path genuine rather than a fresh-database shortcut.
func runMigrationsThrough(t *testing.T, db *sql.DB, through string) {
	t.Helper()

	require.NoError(t, createMigrationsTable(db))

	files, err := getMigrationFiles()
	require.NoError(t, err)
	sort.Strings(files)

	applied := 0
	for _, filename := range files {
		if strings.Compare(filename[:3], through) > 0 {
			break
		}

		content, err := os.ReadFile(filepath.Join("migrations", filename))
		require.NoError(t, err)

		tx, err := db.Begin()
		require.NoError(t, err)
		for _, stmt := range splitSQLStatements(string(content)) {
			_, err := tx.Exec(stmt)
			require.NoError(t, err, "statement in %s", filename)
		}
		_, err = tx.Exec("INSERT INTO schema_migrations (filename, executed_at) VALUES (?, datetime('now'))", filename)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		applied++
	}

	require.Equal(t, 5, applied, "the previous schema is migrations 001-005")
}

// sqliteObjectExists reports whether sqlite_master holds the named object.
func sqliteObjectExists(t *testing.T, db *sql.DB, kind, name string) bool {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, kind, name,
	).Scan(&count))
	return count > 0
}

// retiredSchemaObjects lists every sqlite_master object whose name still
// carries the retired vocabulary.
func retiredSchemaObjects(t *testing.T, db *sql.DB) []string {
	t.Helper()

	rows, err := db.Query(
		`SELECT type || ':' || name FROM sqlite_master WHERE lower(name) LIKE '%' || ? || '%' ORDER BY name`,
		retiredTerm,
	)
	require.NoError(t, err)
	defer rows.Close()

	var found []string
	for rows.Next() {
		var object string
		require.NoError(t, rows.Scan(&object))
		found = append(found, object)
	}
	require.NoError(t, rows.Err())
	return found
}
