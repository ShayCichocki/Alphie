// Package state provides SQLite-based state management for Alphie.
// It handles both global state (~/.local/share/alphie/alphie.db) and
// project-local state (.alphie/state.db).
package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps an SQLite database connection with Alphie-specific operations.
type DB struct {
	conn *sql.DB
	path string
	mu   sync.RWMutex
}

// GlobalDBPath returns the path to the global Alphie database.
func GlobalDBPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "alphie", "alphie.db")
}

// ProjectDBPath returns the path to the project-local database.
func ProjectDBPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".alphie", "state.db")
}

// Open opens an SQLite database at the given path.
// It creates the parent directories if they don't exist.
// WAL mode is enabled for concurrent reads.
func Open(path string) (*DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	db := &DB{
		conn: conn,
		path: path,
	}

	return db, nil
}

// OpenGlobal opens the global Alphie database.
func OpenGlobal() (*DB, error) {
	return Open(GlobalDBPath())
}

// OpenProject opens the project-local database.
func OpenProject(projectRoot string) (*DB, error) {
	return Open(ProjectDBPath(projectRoot))
}

// Close closes the database connection.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.conn.Close()
}

// Path returns the path to the database file.
func (db *DB) Path() string {
	return db.path
}

// Migrate applies all pending schema migrations.
func (db *DB) Migrate() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Create schema version table
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Get current version
	var currentVersion int
	row := db.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	// Apply migrations
	migrations := []struct {
		version int
		sql     string
	}{
		{1, migrationV1Sessions},
		{2, migrationV2Agents},
		{3, migrationV3Tasks},
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := db.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration v%d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration v%d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d: %w", m.version, err)
		}
	}

	return nil
}

// Migration SQL statements
const migrationV1Sessions = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	root_task TEXT NOT NULL,
	tier TEXT NOT NULL,
	token_budget INTEGER NOT NULL DEFAULT 0,
	tokens_used INTEGER NOT NULL DEFAULT 0,
	started_at DATETIME NOT NULL,
	status TEXT NOT NULL DEFAULT 'active'
);

CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
`

const migrationV2Agents = `
CREATE TABLE IF NOT EXISTS agents (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	worktree_path TEXT,
	pid INTEGER,
	started_at DATETIME,
	tokens_used INTEGER NOT NULL DEFAULT 0,
	cost REAL NOT NULL DEFAULT 0.0,
	ralph_iter INTEGER NOT NULL DEFAULT 0,
	ralph_score INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_agents_task_id ON agents(task_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
`

const migrationV3Tasks = `
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	parent_id TEXT,
	title TEXT NOT NULL,
	description TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	depends_on TEXT,
	assigned_to TEXT,
	tier TEXT,
	created_at DATETIME NOT NULL,
	completed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to);
`

// Exec executes a query that doesn't return rows.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.conn.Exec(query, args...)
}

// Query executes a query that returns rows.
func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.conn.Query(query, args...)
}

// QueryRow executes a query that returns at most one row.
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.conn.QueryRow(query, args...)
}

// Transaction runs the given function within a transaction.
func (db *DB) Transaction(fn func(tx *sql.Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// formatTime formats a time.Time for SQLite storage.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// parseTime parses a time string from SQLite.
func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// parseNullableTime parses a nullable time string from SQLite.
func parseNullableTime(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil
	}
	return &t
}

// PurgeOldSessions deletes sessions older than the specified duration.
// Returns the number of sessions deleted.
func (db *DB) PurgeOldSessions(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	cutoffStr := formatTime(cutoff)

	result, err := db.Exec(`
		DELETE FROM sessions WHERE started_at < ?
	`, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("purge old sessions: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}

	return count, nil
}
