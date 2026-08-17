package migrations

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Run runs all migrations in the migrations directory
func Run(db *sql.DB) error {
	// Get all migration files
	migrationFiles, err := getMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to get migration files: %w", err)
	}

	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(db); err != nil {
		return err
	}

	// Get already executed migrations
	executed, err := getExecutedMigrations(db)
	if err != nil {
		return err
	}

	// Execute pending migrations
	for _, filename := range migrationFiles {
		if _, alreadyExecuted := executed[filename]; alreadyExecuted {
			continue
		}

		// Execute migration
		content, err := os.ReadFile(filepath.Join("migrations", filename))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", filename, err)
		}

		// Split into individual statements
		statements := splitSQLStatements(string(content))

		// Execute in transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", filename, err)
		}

		for _, stmt := range statements {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to execute statement in migration %s: %w", filename, err)
			}
		}

		// Record migration execution
		if _, err := tx.Exec("INSERT INTO schema_migrations (filename, executed_at) VALUES (?, datetime('now'))", filename); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", filename, err)
		}
	}

	return nil
}

// getMigrationFiles gets all SQL migration files sorted by name
func getMigrationFiles() ([]string, error) {
	files, err := os.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var migrationFiles []string
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}
		migrationFiles = append(migrationFiles, file.Name())
	}

	// Sort by filename (numerical order)
	sort.Strings(migrationFiles)
	return migrationFiles, nil
}

// createMigrationsTable creates the schema_migrations table
func createMigrationsTable(db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT NOT NULL UNIQUE,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

// getExecutedMigrations gets a map of filename -> true for already executed migrations
func getExecutedMigrations(db *sql.DB) (map[string]bool, error) {
	query := "SELECT filename FROM schema_migrations"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	executed := make(map[string]bool)
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, err
		}
		executed[filename] = true
	}

	return executed, nil
}

// splitSQLStatements splits SQL content into individual statements
// Handles semicolons, but ignores them within strings, comments, and BEGIN...END blocks
func splitSQLStatements(content string) []string {
	var statements []string
	var currentStmt strings.Builder
	inString := false
	inComment := false
	inMultiLineComment := false
	inBeginEndBlock := false

	for i := 0; i < len(content); i++ {
		char := content[i]

		// Handle multi-line comments
		if !inMultiLineComment && i+1 < len(content) && char == '/' && content[i+1] == '*' {
			inMultiLineComment = true
			currentStmt.WriteByte(char)
			i++
			continue
		}
		if inMultiLineComment && i+1 < len(content) && char == '*' && content[i+1] == '/' {
			inMultiLineComment = false
			currentStmt.WriteByte(char)
			i++
			continue
		}
		if inMultiLineComment {
			currentStmt.WriteByte(char)
			continue
		}

		// Handle single-line comments
		if !inString && !inMultiLineComment && char == '-' && i+1 < len(content) && content[i+1] == '-' {
			inComment = true
			currentStmt.WriteByte(char)
			continue
		}
		if inComment && char == '\n' {
			inComment = false
		}
		if inComment {
			currentStmt.WriteByte(char)
			continue
		}

		// Handle strings
		if !inComment && !inMultiLineComment && char == '\'' {
			inString = !inString
		}
		if inString {
			currentStmt.WriteByte(char)
			continue
		}

		// Handle BEGIN...END blocks
		if !inString && !inComment && !inMultiLineComment {
			if isBeginKeyword(content, i) {
				inBeginEndBlock = true
			}
			if isEndKeyword(content, i) {
				inBeginEndBlock = false
			}
		}

		// Handle semicolon (end of statement)
		if char == ';' {
			// Only treat semicolon as statement terminator if we're not in a BEGIN...END block
			if !inBeginEndBlock {
				currentStmt.WriteByte(char)
				stmt := strings.TrimSpace(currentStmt.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				currentStmt.Reset()
			} else {
				currentStmt.WriteByte(char)
			}
		} else {
			currentStmt.WriteByte(char)
		}
	}

	// Add the last statement if there's any content left
	if currentStmt.Len() > 0 {
		stmt := strings.TrimSpace(currentStmt.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

// isBeginKeyword checks if the current position starts with the BEGIN keyword
func isBeginKeyword(content string, pos int) bool {
	if pos+4 >= len(content) {
		return false
	}
	// Check for BEGIN (case insensitive, but content should be uppercase)
	return strings.HasPrefix(strings.ToUpper(content[pos:]), "BEGIN")
}

// isEndKeyword checks if the current position starts with the END keyword
func isEndKeyword(content string, pos int) bool {
	if pos+2 >= len(content) {
		return false
	}
	// Check for END (case insensitive, but content should be uppercase)
	return strings.HasPrefix(strings.ToUpper(content[pos:]), "END")
}
