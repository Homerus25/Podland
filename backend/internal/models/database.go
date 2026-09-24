package models

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// NewDatabase creates a new database connection
func NewDatabase(path string) (*sql.DB, error) {
	// _txlock=immediate makes every transaction on this database open
	// with BEGIN IMMEDIATE, so a transaction takes SQLite's single
	// writer lock at BEGIN instead of at its first write. Check-then-
	// write transactions (session Rotate, the first-user creation guard)
	// are then serialized: the loser waits on busy_timeout and re-reads
	// committed state, instead of racing a stale snapshot into a
	// SQLITE_BUSY error the caller cannot distinguish from a storage
	// failure.
	db, err := sql.Open("sqlite3", path+"?_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set busy timeout
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	return db, nil
}
